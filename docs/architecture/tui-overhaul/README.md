# TUI Overhaul — Transcript-First Operator Application

- Status: Draft for operator review
- Baseline date: 2026-08-27
- Reference implementation: local `openai/codex` checkout at
  `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`
- Implementation status: E0 through E2 are complete; TUI-030 is the only
  pending canonical task, and later tasks remain unpublished drafts
- Epic and task index: [Implementation plan](#implementation-plan)

## Purpose

Replace Revolvr's dashboard-and-pages presentation with a terminal application
that feels like the Codex CLI: a durable transcript, one live operation area,
an always-available composer, and temporary overlays for focused actions.

This is a TUI architecture and interaction change, not a replacement runtime.
Revolvr's Go application services, safety policy, scheduler, ledger, receipts,
artifacts, and task lifecycle remain authoritative.

The implementation epics and tasks remain drafts until published by a
completion handoff. TUI-010 through TUI-022 are complete and TUI-030 is the
only pending canonical task; all later tasks remain drafts. This document
captures the current system, the accepted product decisions and source
snapshots, and a bounded implementation sequence.

## Plan Use and Task Contract

The files under `epics/` and `tasks/` are the only draft implementation plan.
They are not canonical tasks and must not be copied wholesale into
`.agent/tasks/` or `.agent/TASKS.md`.

Each task owns one decision, proof, component change, view migration, or
verification pass. Its tests and documentation of that same outcome are part
of its definition of done, not separate work. If implementation exposes a
missing app/domain capability, add one bounded prerequisite task rather than
absorbing that capability into a TUI task.

Each fresh pass executes one bounded decision, proof, or implementation task.
After it is terminally complete and verified, its completion handoff may
promote exactly one accepted, dependency-satisfied next draft as pending. That
publication is metadata for the completed pass, never a standalone pass; the
next fresh pass implements the pending task instead of republishing it.

Recovery only: when the canonical selector is empty and `.agent/HANDOFF.md`
names one exact accepted, dependency-satisfied draft, publish it and continue
directly into that task in the same pass. If the next draft is ambiguous,
blocked, or needs an unresolved product decision, record the blocker and
publish nothing. The canonical selector and dependency checks remain
authoritative. Never bulk-publish the draft backlog or execute more than one
product task per pass. Combine adjacent draft tasks only when the code proves
they are one smaller change; do not combine them for scheduling convenience.

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

- the old dashboard is still a Bubbles viewport migration panel above a
  footer;
- the dashboard reconstructs the latest run into a fixed page on every draw;
- the composer is an inactive footer affordance until `/` is pressed;
- Tasks, Runs, Run Detail, Preflight, Workflow, Help, Change Summary, Evidence,
  and Approval are page-like views selected in the main model;
- live progress is prepended to the selected page rather than being one live
  transcript cell;
- only the startup session cell currently uses terminal-owned history.

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
- `StatusModel.Init` appends one source-backed `session-start` through
  `tea.Println` before managed content.
- `StatusModel.View` renders the migration viewport, a blank row, and the
  existing footer; it has no persistent header.
- Bubbles `viewport.Model` owns the central content area's dimensions and
  scrolling. The viewport height is derived from the current footer row count.
- Lip Gloss is applied after plain-text wrapping. Primary text uses the
  terminal default, headings are bold, secondary text is faint, selection is
  ANSI cyan, success is ANSI green, and failure is ANSI red.

### Dashboard

- The one-time session cell owns `Revolvr`, the inspected project root, and the
  initialization fact at process start.
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

### Accepted experience-state snapshots

**Status: Accepted 2026-08-27.** These source snapshots are the presentation
authority for TUI-010 through TUI-072. Text to the right of `│` is literal
visible content. The label to the left is a documentation annotation and is
not rendered:

- `session` — the committed `session-start` cell;
- `transcript` — a committed canonical-history cell;
- `live` — the one replaceable current-state cell;
- `composer` — editable input or its empty-buffer prompt;
- `overlay` — the focused workflow that temporarily owns input;
- `footer` — focus keys or one ephemeral action result.

Blank layout rows carry no fact and are not part of the source contract. No
visible fact appears under more than one owner in a snapshot.

#### Initialized and idle — 80 columns

```text
session    │ Revolvr
session    │ Project: /home/alex/source/revolvr
session    │ At start: initialized
live       │ Ready
live       │ Next task: Compact durable agent state
live       │ Next: type a task or use /run
composer   │ ›
footer     │ Enter submit · / commands · ? shortcuts
```

`At start: initialized` is immutable process-start history. `Ready`, the next
task, and the next action are one replaceable current projection; refresh may
replace them without rewriting the session cell.

#### Uninitialized — 80 columns

```text
session    │ Revolvr
session    │ Project: /home/alex/source/revolvr
session    │ At start: not initialized
live       │ Not initialized
live       │ Next: run revolvr init in this repository
composer   │ ›
footer     │ Enter submit · / commands · ? shortcuts
```

Submitting nonblank plain text in this state preserves the buffer and replaces
the composer-owned refusal with the literal text `Input unavailable: run
revolvr init first`. It calls no application service. Startup inspection or
status failure emits no session cell and launches no TUI.

#### Running — 80 columns

```text
session    │ Revolvr
session    │ Project: /home/alex/source/revolvr
session    │ At start: initialized
live       │ Running: Compact durable agent state
live       │ Mode: loop · pass 1 of 3
live       │ Safety: admitted
live       │ Current: Running go test ./...
live       │ Next: wait, or press c or Esc to cancel
composer   │ ›
footer     │ Enter submit · / commands · ? shortcuts
```

The live cell is replaced in place; progress does not append rows. Active plain
text remains local and is rejected with `Input unavailable: active steering is
not supported`. After cancellation is requested, the same live owner renders
exactly:

```text
live       │ Cancelling: Compact durable agent state
live       │ Current: waiting for the run to stop
live       │ Next: wait for settlement
```

The program neither clears this cell nor exits until the matching domain result
settles.

#### Completed — 80 columns

```text
session    │ Revolvr
session    │ Project: /home/alex/source/revolvr
session    │ At start: initialized
transcript │ Completed: Compact durable agent state
transcript │ Verification: passed
transcript │ Commit: ff50d9b5cd07
transcript │ Next: /run to continue
composer   │ ›
footer     │ Enter submit · / commands · ? shortcuts
```

#### Failed — 80 columns

```text
session    │ Revolvr
session    │ Project: /home/alex/source/revolvr
session    │ At start: initialized
transcript │ Failed: Compact durable agent state
transcript │ Reason: verification failed
transcript │ Detail: go test ./... exited 1
transcript │ Next: /detail to inspect the failure
composer   │ ›
footer     │ Enter submit · / commands · ? shortcuts
```

#### Cancelled — 80 columns

```text
session    │ Revolvr
session    │ Project: /home/alex/source/revolvr
session    │ At start: initialized
transcript │ Cancelled: Compact durable agent state
transcript │ Result: no completion was recorded
transcript │ Next: /run to retry
composer   │ ›
footer     │ Enter submit · / commands · ? shortcuts
```

#### Needs-input question — 80 columns

```text
session    │ Revolvr
session    │ Project: /home/alex/source/revolvr
session    │ At start: initialized
transcript │ Needs input: task-017
transcript │ Question: Choose the verification scope
transcript │ Next: answer the question to continue
overlay    │ Answer required
overlay    │ Task: task-017
overlay    │ Choose one option
overlay    │ > focused — Run package tests
overlay    │   full — Run all tests
overlay    │ Enter reviews the selected answer
footer     │ j/k choose · Enter review · Esc back
```

The normal composer is absent while the typed question owns focus. After the
first Enter, the focused fragment is exactly:

```text
overlay │ Confirm answer
overlay │ focused — Run package tests
footer  │ Enter submit · Esc options
```

Only the second Enter calls `app.AnswerAutonomousInput` with the exact typed
identity.

#### Help overlay — 80 columns

```text
session    │ Revolvr
session    │ Project: /home/alex/source/revolvr
session    │ At start: initialized
overlay    │ Help
overlay    │ ?  Help
overlay    │ /help  Help
overlay    │ /tasks  Tasks
overlay    │ /runs  Runs
overlay    │ /detail  Run Detail
overlay    │ Esc closes Help
footer     │ ↑/↓ scroll · Esc close
```

The overlay owns focus and its local scroll state. It emits no transcript row;
dismissal restores the exact composer buffer against the latest live state.

#### Running — 40-column minimum

The visible content below is at most 40 display columns per row; the ownership
gutter is not rendered or counted.

```text
session  │ Revolvr
session  │ Project: /home/alex/source/revolvr
session  │ At start: initialized
live     │ Running: Compact durable agent state
live     │ Mode: loop · pass 1 of 3
live     │ Safety: admitted
live     │ Current: Running go test ./...
live     │ Next: wait, or press c or Esc to cancel
composer │ ›
footer   │ Enter submit · / commands
footer   │ ? shortcuts
```

#### Exact terminal-result vocabulary

The completed, failed, cancelled, and needs-input snapshots above are literal
fixtures. The remaining terminal results use these literal committed cells:

```text
transcript │ Blocked: task-017
transcript │ Reason: dependency task-016 is pending
transcript │ Next: /workflow to inspect the task

transcript │ Safety stop: task-017
transcript │ Reason: protected path changed
transcript │ Next: /detail to inspect the evidence
```

`Safety: admitted` is active state only. `Safety stop:` is a terminal outcome;
neither color nor an icon supplies either meaning.

#### Width, wrapping, and truncation contract

- Normal acceptance geometry is 80x24. The minimum supported geometry is
  40x24. Width assertions use ANSI-stripped terminal display cells, not bytes
  or rune count.
- Labels, safety, cancellation, terminal outcome, `Next:`, and the selected
  typed option are never truncated or removed. Prose, errors, questions, and
  the exact project root wrap with a two-column hanging indent; an unbroken
  token hard-wraps rather than overflowing.
- `Current:` detail may occupy two physical rows. Additional detail is replaced
  by a trailing `…`; complete canonical evidence remains in Run Detail or its
  owning overlay. Secondary overlay lists scroll instead of growing the frame.
- The footer wraps only between key/action items. At 40 columns the owning
  action remains before discovery hints, as shown above.
- At widths from 1 through 39, Revolvr remains best-effort usable but support is
  not claimed: content width clamps to one display cell, required state and
  action text wraps vertically, secondary live detail may reduce to `…`, and
  overlay content scrolls. The application never clears or re-emits committed
  terminal rows to compensate for the narrow terminal.

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

**Status: Accepted 2026-08-27.** Use a bounded hybrid. Existing application
services remain canonical; `internal/tui` retains a bounded set of semantic
committed source cells and one replaceable live cell. New finalized cell
renderings are appended exactly once above the inline Bubble Tea program using
the installed `tea.Println` boundary. The normal managed frame contains only
the live cell, composer, and any active overlay. A Bubbles viewport may scroll
an overlay, but it does not own committed transcript history.

Already-emitted rows are a terminal-owned presentation cache. Revolvr does not
clear or reinsert them on resize and does not add a terminal backend or escape
layer. Retained source cells, not terminal rows, remain the replay and identity
source. Stable run, event, operation, question, or approval identity reconciles
live and committed meaning; timestamps and rendered prose never do.

| Concern | Owner | Source of truth | Allowed presentation cache | Rebuild or replay | Failure or fallback | Proof obligation |
| --- | --- | --- | --- | --- | --- | --- |
| Canonical application meaning | Existing app/domain services | Tasks, ledger, `app.RunTimeline`, artifacts, and typed operation results | None in the TUI may replace it | Reload through existing callbacks | Retain the last good projection and show the app error | TUI-021, TUI-022 |
| Semantic committed source cells | `internal/tui` | Bounded TUI projection of canonical meaning plus stable domain identity | Width-keyed rendered lines may be cached | Rebuild deterministically on startup and refresh | Unknown or malformed input remains visible and non-successful | TUI-020, TUI-021 |
| Rendered committed lines | Normal-screen terminal history, fed by Bubble Tea's standard renderer | The corresponding semantic source cell at the emission width | Terminal rows and a per-session set of emitted cell identities | Emit each identity once; never reconstruct meaning from rows | If append-above-program fails, do not install the shell; reopen D3 | TUI-010 |
| Replaceable live cell | `internal/tui.StatusModel` | Current typed operation projection and stable operation token/identity | Current width-aware rendered rows only | Redraw from live source on every update and resize | Preserve a textual error/terminal result until canonical reconciliation succeeds | TUI-010, TUI-012, TUI-040 |
| Composer and overlay focus | `internal/tui.StatusModel` | Explicit composer, typed-input, and overlay state | Local buffer, selection, scroll, and prior-focus state | Redraw in place; restart discards ephemeral input state | Active modal owner keeps focus; failed close leaves the overlay visible | TUI-010, TUI-050 |
| Scroll position and copy/paste history | Terminal or multiplexer | Its normal-screen history buffer; it is not semantic authority | Native scroll position and selection | Revolvr performs no scroll-offset replay | Canonical evidence remains reachable through focused views; unsupported terminal behavior must be recorded | TUI-061 |
| Managed-frame wide-to-narrow and narrow-to-wide reflow | `internal/tui` | Retained source cells, current live state, composer/overlay state, and current width | Current rendered frame | Re-render at the new width without changing identity | Clamp to one usable column and follow accepted narrow snapshots | TUI-011, TUI-060 |
| Previously emitted-row resize/reflow | Terminal or multiplexer | Existing terminal rows | Native soft-wrap/reflow only | Never clear, rewrap, or re-emit from the application | A terminal limitation is documented with a workaround or marked unsupported; no duplicate replay | TUI-061 |
| Live-to-committed settlement | `internal/tui.StatusModel` | Stable operation identity reconciled with the canonical terminal result | Per-session emitted-identity set | Clear the live cell only as its final semantic cell is emitted once | Keep the settled live result visible if canonical refresh is unavailable | TUI-012, TUI-022 |
| Refresh reconstruction | `internal/tui.StatusModel` | Fresh canonical projection | Retained source cells and emitted-identity set for this process | Replace source projection and emit only newly discovered identities | On refresh error, retain prior cells and report the error | TUI-021, TUI-022 |
| Restart reconstruction | `internal/tui.StatusModel` | Fresh canonical projection | A new process-local emitted-identity set | Replay the accepted bounded history once for the new program session | Do not guess what an earlier process left in terminal history; D6 decides the session marker | TUI-004, TUI-021 |
| Overlay open/close return | `internal/tui.StatusModel` | Explicit prior focus, composer buffer, overlay identity, and live operation identity | Overlay-local selection and viewport offset | Leave terminal history untouched; redraw the latest live/composer frame on close | Keep the overlay open on failed action; Escape restores the exact prior local state | TUI-050, TUI-061 |
| Plain terminal and tmux behavior | Bubble Tea plus the terminal or tmux | Same semantic cells; environment owns the rendered history | Native normal-screen scrollback | Append once through the installed renderer | Support is claimed only after the recorded plain-terminal/tmux matrix passes | TUI-010, TUI-061 |
| Non-TTY and test output | Bubble Tea's configured output writer | Same semantic cells and live state | Captured renderer bytes only | Produce deterministic committed lines and a final managed frame without native-navigation claims | A failing buffer proof blocks shell installation; no TTY behavior is inferred | TUI-010 |
| Initialization, normal exit, and error restoration | `tea.Program` | Installed Bubble Tea lifecycle | Renderer-owned terminal modes | Use existing startup/teardown; add no Revolvr escape layer | Any restoration gap requires a focused fix proven before support is claimed | TUI-010, TUI-062 |
| Cancellation and quit settlement | `internal/tui.StatusModel` | Existing cancellation context and matching domain result | Replaceable live terminal state | Wait for settlement, emit the final cell once, then return `tea.Quit` | Never erase the live cell or report exit before the matching result | TUI-012, TUI-062 |

Rejected alternatives:

- **Terminal-native ownership with application-driven reflow** would require
  clearing and reinserting terminal history or a new escape/terminal layer.
  `tea.Println` proves append-only persistence, not portable reflow, replay, or
  tmux behavior.
- **Viewport-owned committed history** keeps redraw and resize simple but hides
  the transcript behind application scrolling and selection, duplicating the
  current viewport shell instead of providing native copy-friendly history.

TUI-010 proves append-above-program composition in two test output buffers and
one real terminal. TUI-011 proves source/live reflow without
re-emitting committed identities; TUI-012 proves settlement and restoration.
TUI-061 later records native scroll, copy, resize, and tmux behavior, while
TUI-062 records normal, cancellation, and error restoration. A failed
append/composition proof reopens D3 before TUI-013; unproven terminal-specific
behavior remains an explicit limitation rather than a reason to add machinery.

### D4 — Overlay migration

**Status: Accepted 2026-08-27.** Migrate exactly one focused workflow at a time
in this order: Help; Tasks; Runs with its Run Detail child; Preflight;
Workflow; Change Summary; Evidence; Approval; typed needs-input. TUI-050
through TUI-058 encode that order as a dependency chain. A migration does not
start until the preceding view's parity gate passes.

The operator-facing entry routes below are retained throughout E5. Before a
view's migration passes, its key and command continue to open the current page.
At the accepted cutover, both routes open the same overlay state. The old page
implementation then remains as a rollback path until TUI-070; D4 does not
authorize removing any listed key or command.

| Order | Task and focused workflow | Retained key and command entry | View-specific parity gate |
| --- | --- | --- | --- |
| 1 | TUI-050 — Help and the shared shell | `?`; Enter on bare `/`; `/help`; `/commands` | All current Help content and action descriptions render in the overlay; every retained entry opens it; Escape restores the exact composer buffer/focus; 80- and 40-column scroll/resize checks pass; a live settlement behind Help is correct on return. |
| 2 | TUI-051 — Tasks | `2`; `/tasks` | Task list/detail, stable selection across refresh, Add Task, retry, open-Workflow, confirmations, guards, and success/failure presentation match the page path; key and command entry plus exact return are tested. |
| 3 | TUI-052 — Runs and Run Detail | `3`; `/runs`; `4`; `/detail`; Runs `Enter`/`o` opens detail | Runs selection/scroll survives refresh by run identity; both direct-detail entries and parent selection reach the same child; detail content, scrolling, receipt validation, warnings, artifacts, and raw audit/debug evidence match; child back and root dismissal are tested. |
| 4 | TUI-053 — Preflight | `5`; `/preflight` | Pass, warning, and refusal projections match; `p`, refresh, run actions, unavailable callbacks, and active-operation guards retain their behavior; both entries and exact dismissal return are tested. |
| 5 | TUI-054 — Workflow | `6`; `/workflow` | Every current lifecycle, selector, scroll position, control, live update, guard, error, and needs-input indication remains reachable; the existing typed-answer route still works before TUI-058; both entries and return are tested. |
| 6 | TUI-055 — Change Summary | `d`; `/diff` | Changed-file and commit metadata, exact-diff distinction, scrolling, refresh, compaction, guards, and source traceability match; both entries and return are tested. |
| 7 | TUI-056 — Evidence | `e`; `/evidence` | Evidence groups, stable selection, statuses, warnings, artifact references, validation/action routes, refresh, compaction, and traceability match; both entries and return are tested. |
| 8 | TUI-057 — Approval | `A`; `/approval` | Exact request/evidence identity, selection, confirmation, refusal, stale-result, refresh, active-operation, error, and no-unconfirmed-effect behavior match; both entries and return are tested. |
| 9 | TUI-058 — typed needs-input | context-specific `a` from Workflow or Approval; `/answer <option-id>` | Exact task/question/revision/content/option identity, explicit selection, two-step confirmation, stale-result refusal, failure recovery, cancellation, and no-free-form-answer behavior match; both entries and parent-overlay return are tested at narrow height and width. |

`internal/tui.StatusModel` owns overlay focus and return state. Opening a root
overlay records its identity and the current composer buffer, cursor/focus,
and originating live-operation identity. Overlay-local selection, scroll,
confirmation, and error state belong to that overlay. The committed-cell
source, emitted identities, and live operation continue to update underneath
it rather than being snapshotted. Root Escape dismisses without emitting a
transcript row or moving terminal history and redraws the latest underlying
live state with the exact saved composer state. A failed action keeps the
owning overlay visible; a stale result cannot dismiss or replace a newer
overlay.

There is no general overlay stack. Runs has one explicit child state: opening
Run Detail retains the selected run identity and Runs list offset; Escape or
Backspace from detail returns to that exact Runs parent, and a second Escape
dismisses the root. Direct `4` or `/detail` entry creates the same Runs parent
state before showing the current detail or existing empty-detail result. Typed
needs-input is the other explicit modal child: it retains Workflow or Approval
as its parent. Cancellation returns to that exact parent; a failed or stale
submission keeps the child visible and leaves the parent unchanged.

A page-only implementation is removable only in TUI-070, after all nine E5
parity gates pass, E6 geometry checks cover its overlay, tests prove every
listed key and command reaches the same accepted overlay behavior, dismissal
and child-back tests prove exact return, and a review finds no fact, action,
guard, or error path unique to the page. TUI-070 may then delete only page
selection/rendering and migration scaffolding; the listed operator entries,
app callbacks, projections, guards, and domain state remain. Until those
criteria pass, rollback is the small routing change back to the retained page,
not a second domain or callback path.

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

**Status: Accepted 2026-08-27.** Replace the persistent
`Revolvr Dashboard initialized` row with one immutable committed session cell
at the start of each TUI process. Its process-local presentation identity is
`session-start`; it is emitted once before the accepted bounded canonical
history replay. A restarted process has a new emitted-identity set, so it emits
its own `session-start` cell and then rebuilds the bounded history without
guessing what an earlier process left in terminal scrollback.

The session cell contains exactly these facts:

| Displayed fact | Exact source | Presentation owner |
| --- | --- | --- |
| Product label `Revolvr` | A package-local `internal/tui` presentation constant | Committed session cell |
| Project identity | Initial `app.StatusResult.ProjectRoot`, projected by `app.Status` from `repositorypath.Inspect(...).Root()` for its `app.Config.WorkDir`; the TUI does not read the ambient working directory, Git remote, or repository basename | Committed session cell |
| State at process start, `initialized` or `not initialized` | The initial `app.StatusResult.Initialized`, derived by `repositorypath.Authority.Initialized()` from the inspected `.revolvr` directory and ledger presence | Committed session cell |

The state wording must make its point-in-time scope clear; a later refresh
does not rewrite terminal history. The cell contains no current view, ready or
safety claim, active mode, task/run identity, workflow, count, timestamp,
version, command hint, or error. Those values either change during the process
or have no evidenced operator need in session history.

Presentation ownership for the remaining visible facts is exclusive:

| Fact class | Presentation owner | Source |
| --- | --- | --- |
| Stable historical operator/run meaning | Committed transcript cell | Canonical tasks, ledger, `app.RunTimeline`, artifacts, and typed app results under D3 |
| Active operation identity, mode, progress, cancellation, and unsettled result | Replaceable live cell | Existing typed operation state and callbacks |
| Focused workflow content, selection, confirmation, and owning error | Overlay | Existing callback-backed projection plus overlay-local state under D4 |
| Editable input, command discovery, task draft, and input refusal | Composer | Composer/task-entry local state plus existing command guards under D2/D5 |
| Current focus-appropriate keys and ephemeral refresh/action acknowledgement or failure | Transient footer | `StatusModel` focus/action result state; it repeats no session or lifecycle fact |

The lifecycle is fixed and snapshot-testable:

- **Startup:** after the initial status and repository-root inspection succeed,
  emit `session-start` once, then emit the accepted bounded canonical history.
  A startup inspection/status failure prevents TUI launch and emits no cell.
- **Refresh:** retain the session source and emitted identity. Apply the fresh
  status to current guards, append only newly discovered canonical identities,
  and report refresh success/failure transiently. A failed refresh retains the
  last good projection. It never emits or mutates the session cell.
- **Resize:** redraw only retained managed state at the new width. Never
  re-render or re-emit `session-start`; the terminal owns any reflow of its
  already-emitted rows.
- **Restart:** create a new process-local emitted set, emit one new
  `session-start` from the new initial projection, and replay the bounded
  canonical history once for that process.
- **Explicit clear:** the overhaul adds no clear key, command, callback, or
  presentation epoch. If an operator clears terminal or multiplexer scrollback
  outside Revolvr, the application neither detects it nor emits another
  session cell. A future Revolvr-owned clear requires a separate bounded
  product decision and task.
- **Overlay open/dismiss:** opening or dismissing an overlay changes no session
  source or emitted identity and emits no history. The overlay restores the
  latest live/composer frame under D4.

Identity and deduplication use typed source identity, never timestamp or
rendered prose. The process-local emitted set records `session-start` in a
session-cell namespace distinct from canonical run/event/operation identities.
It is recorded only at the append boundary and is not cleared by refresh,
resize, or overlay transitions. A Bubble Tea output failure fails the program
through its normal lifecycle and must not be reported as a successful header;
refresh and resize never attempt a blind retry.

TUI-013 removed persistent header chrome after the installed path proved one
`session-start` before managed content and zero additional session cells across
refresh, navigation, and resize. TUI-060 and TUI-062 retain the broader
restart, final-geometry, and terminal-restoration gates before TUI-070 removes
the remaining dashboard/footer presentation. No app/domain capability,
callback, domain authority, runtime dependency, terminal backend, or clear
action was added; TUI-013 only added the root to the existing status
projection.

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

**Accepted 2026-08-27.** D1-D6 and the experience-state snapshots are
consistent, the 33-task epic index remains bounded, and E0 has exited. No
implementation task was published or started by the review.

TUI-010 through TUI-022 are complete. TUI-022's completion handoff published
only [TUI-030](tasks/tui-030-make-composer-primary.md); every later TUI task
remains an unpublished draft.
