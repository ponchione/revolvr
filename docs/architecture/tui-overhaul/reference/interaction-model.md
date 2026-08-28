# Interaction Model

Evidence pin: `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`.
Labels in this document are deliberate: observed behavior describes the pin,
current behavior describes Revolvr today, candidate adaptation is non-binding,
and decision status records accepted D2-D6.

## Shell Composition and Focus

**Observed Codex behavior:** application state retains semantic transcript cells
separately from an optional overlay, while the chat widget owns the mutable
active cell and bottom pane. An application overlay receives events before the
chat widget. ([I01], [I02])

**Observed Codex behavior:** the normal surface is a vertical composition: a
flexible active transcript region, optional transient activity, pending status,
and a fixed bottom pane. The bottom pane renders status and pending-input
previews above a composer. ([I03], [I04])

**Observed Codex behavior:** the bottom pane owns a stack of active views. The
top view receives keys and replaces the normal status/preview/composer rendering;
when no view is active, keys go to the composer and task interruption remains a
higher-priority route. ([I05], [I06])

```text
App event owner
├── application overlay active ──> overlay owns input and draw
└── normal chat surface
    ├── terminal scrollback ─────> committed HistoryCell renderings
    └── inline viewport
        ├── active HistoryCell ──> replaceable live tail/status
        └── BottomPane
            ├── top modal view ─> owns focus while present
            └── normal pane ────> status + pending preview + composer
```

**Accepted Revolvr adaptation:** keep one Bubble Tea model with explicit
committed source cells, one live cell, composer state, and overlay state.
Finalize each stable committed identity once through Bubble Tea's installed
append-above-program boundary; terminal history owns emitted rows while the
managed frame owns the live cell, composer, and overlay. A viewport may scroll
overlay content but never owns committed history.

**Decision status:** D3 accepts this bounded hybrid. D4 accepts a parity-gated,
one-view-at-a-time overlay migration with exact focus return.

## Live-to-Committed Settlement

**Observed Codex behavior:** `HistoryCell` is both the committed transcript unit
and the interface used transiently for an in-flight active cell. Cells render
from semantic/source state, measure wrapped height for a width, and may expose a
different copy-friendly transcript representation. ([I07])

**Observed Codex behavior:** streaming assistant text is held in a controller;
visible tail changes replace the active cell and bump its revision. On finish,
the active tail is cleared and finalization requests consolidation. The app
then replaces the trailing run of provisional assistant cells with one
source-backed markdown cell and can force scrollback reflow. ([I08], [I09])

**Observed Codex behavior:** committing a cell appends it to the retained
transcript source, updates the transcript overlay when open, and renders or
buffers its lines for terminal history. This is the boundary at which mutable
content becomes durable presentation. ([I10], [I11])

**Accepted Revolvr adaptation:** define one settlement operation that matches a
live operation to canonical meaning by stable domain identity, appends the
final semantic cell once, records its identity in the process-local emitted
set, and only then clears the live cell.

**Decision status:** D3 assigns semantic source and settlement to `internal/tui`
and already-emitted rows to terminal history. TUI-012 and TUI-022 still must
prove the exact-once boundary.

## Semantic Cell Categories

**Observed Codex behavior:** the pinned cell modules cover approvals, base text,
execution, hooks, MCP, messages/reasoning, notices, patches, plans, typed-input
results, search, separators, and session information. The interface—not the
Rust type inventory—is the reusable contract: each category supplies semantic
source and width-aware display/transcript lines. ([I07], [I12])

**Accepted Revolvr adaptation:** Revolvr needs only the categories accepted by
TUI-020: the D6 `session-start` cell, operator input, run/task status, progress,
result, warning, question, and approval evidence. Codex-specific categories do
not justify matching Go types one-for-one.

**Accepted Revolvr presentation:** TUI-005 assigns every literal visible row
in the [experience-state snapshots](../README.md#accepted-experience-state-snapshots)
to session, transcript, live, composer, overlay, or transient-footer ownership.
The labels, safety/cancellation/outcome vocabulary, and next actions are
Revolvr source fixtures; no Codex snapshot wording is copied.

## Composer Ownership and Submission

**Observed Codex behavior:** the composer is constructed with input focus and
remains the normal input owner. Submission trims text, suppresses empty input,
restores the draft after unknown-command or size validation failures, and
records successful local history. ([I13])

**Observed Codex behavior:** submit and queue bindings share a dispatch path;
the running state determines whether the configured queue key queues input.
Up/down browse history only at safe textarea boundaries, preserving multiline
cursor movement. ([I14], [I15])

**Current Revolvr behavior:** the composer is a two-field state activated only
by `/`; it accepts runes, space, backspace, escape, and enter, then dispatches
slash commands. Plain text has no accepted meaning. (Revolvr
`internal/tui/model.go:195-198,515-528,1845-1889`
(`commandComposerState`, `Update`, `updateCommandComposer`, `submitCommand`))

**Accepted Revolvr adaptation:** make the composer the normal focus. Initialized
idle plain text opens the existing reviewed Add Task flow; every other
plain-text state rejects without dispatch and preserves the buffer. Slash
commands retain their current app paths and guards.

**Accepted product decision:** D2 gives plain text only the reviewed idle task-
draft meaning. D5 rejects current-process steering and later-pass or queue
input. Codex demonstrates one coherent chat model but does not override
Revolvr’s narrower authority.

## Commands and History

**Observed Codex behavior:** typing a leading slash filters a selectable command
list by exact and prefix match, omits duplicate aliases from the default list,
and resets/clamps selection when the filter changes. ([I16])

**Observed Codex behavior:** local history ignores empty entries, collapses
adjacent duplicates, and supports older/newer traversal. Non-empty drafts enter
history navigation only when they match the last recalled entry and the cursor
is at a boundary. ([I15])

**Candidate Revolvr adaptation:** TUI-032 can add contextual command discovery
against Revolvr’s existing command vocabulary. Persistent cross-session prompt
history is not required by the accepted plan.

## Queued Input and Interruption

**Observed Codex behavior:** queued messages, submitted-but-uncommitted steers,
and rejected steers are distinct state and distinct preview categories. A new
idle submission is optimistically rendered before remote turn-start latency;
during an active turn it becomes a pending steer instead of a committed user
cell. ([I17], [I18])

**Observed Codex behavior:** approvals, permission prompts, typed questions, and
selected tool lifecycle events are serialized in an interrupt queue and replayed
in order after the current interrupt. ([I19])

**Accepted Revolvr adaptation:** render no queued operator input. Revolvr has no
application/domain service for its lifecycle or identity, and a TUI-only queue
would create a second workflow authority. `autonomousqueue` orders canonical
tasks, not operator messages.

**Accepted product decision:** D5 rejects active steering and queued/deferred
input. TUI-041 is removed and no domain prerequisite is created.

## Overlays and Focus Transfer

**Observed Codex behavior:** an application-level overlay preempts the chat
surface entirely, while bottom-pane modal views preempt composer input through
a view stack. Completing or cancelling a child pops it and returns ownership to
the preceding view or normal pane. ([I02], [I05], [I06])

**Observed Codex behavior:** approval and request-for-input events first flush
streaming answer and completed command activity, then install a modal and request
a redraw. The composer is explicitly disabled while a typed question view is
active. ([I20], [I21])

**Accepted Revolvr adaptation:** TUI-050 introduces one root overlay shell with
explicit focus return. The accepted migration order is Help, Tasks, Runs/Run
Detail, Preflight, Workflow, Change Summary, Evidence, Approval, then typed
needs-input. Each step retains its current key and command, cuts both entries
over only after content/action and return-state parity passes, and leaves the
page renderer as rollback scaffolding until the D4 removal criteria pass.

Runs-to-Run-Detail and typed needs-input are the only accepted child states,
not a general overlay stack. A root owns its local selection/scroll/error state
and saved composer/source return state while current transcript/live meaning
continues underneath. Root dismissal restores exact composer focus/buffer
against the latest live state without emitting history. A failed action keeps
the owner visible, and stale results cannot dismiss a newer overlay.

**Accepted product decision:** D4 fixes that order and the per-view parity,
retained-entry, child-back, exact-return, and page-removal gates. It changes no
application callback or domain state.

## Typed Questions

**Observed Codex behavior:** the question overlay owns a request queue,
per-question answer state, current index, focus, and a dedicated instance of the
shared composer configured without slash popups. Options and per-question notes
can coexist; submit advances questions and the last question sends all answers.
([I22], [I23])

**Observed Codex behavior:** submission constructs answers keyed by question id,
emits the protocol response, inserts a semantic result cell, and advances to the
next queued request or completes. The snapshots exercise options, notes, tight
height, wrapped options, long options, and footer wrapping. ([I24], [I25])

**Current Revolvr behavior:** typed needs-input already rejects absent or stale
questions, begins with no recommendation selected, requires explicit selection
and two-step confirmation, checks question identity again before persistence,
and calls an application callback. (Revolvr
`internal/tui/model.go:1050-1128` (`beginAutonomousAnswer`,
`updateAutonomousAnswer`); `internal/tui/autonomous_test.go:101-181`
(`TestAutonomousAnswerRequiresExplicitChoiceAndDoubleConfirmation`,
`TestAutonomousAnswerRejectsReloadedQuestionDuringConfirmation`))

**Candidate Revolvr adaptation:** migrate that typed state machine into the
shared overlay shell; do not replace it with unstructured composer text.

**Accepted product decision:** D2 rejects free-form answers. TUI-058 retains the
exact typed option and confirmation path.

## Approvals

**Observed Codex behavior:** the approval overlay converts a typed request into
typed options, applies at most one selection to the current request, emits a
semantic decision cell for ordinary requests, and sends the decision using the
request identity. Additional requests queue behind the current one. ([I26])

**Observed Codex behavior:** cancellation maps to a typed negative/cancel
decision, clears the queue, and completes the view. A separately resolved stale
request closes only when identity matches and emits no extra decision; direct
tests cover both cases. ([I27], [I28])

**Current Revolvr behavior:** Approval is a focused full-screen presentation of
the canonical autonomous projection; its needs-input answer route uses the
typed callback described above. (Revolvr `internal/tui/model.go:2038-2062,2613-2683`
(`openFocusedView`, `renderFocusedApproval`);
`internal/tui/architecture_024_test.go:122-188`
(`TestApprovalComposerSubmitsTypedNeedsInputResponse`))

**Candidate Revolvr adaptation:** keep application identity and persistence
checks intact while changing only focus and presentation ownership.

## Cancellation

**Observed Codex behavior:** cancellation is contextual: an active modal gets
the event first; approval cancellation resolves its current request and closes
the view; absent a view, a configured task-interrupt action can preempt composer
handling. ([I06], [I27])

**Current Revolvr behavior:** an active run intercepts quit/cancel keys, invokes
its context cancel function once, records cancellation state, and can delay quit
until settlement. (Revolvr `internal/tui/model.go:1524-1577`
(`updateActiveRunKeys`, `requestRunCancel`);
`internal/tui/model_test.go:1234-1300`
(`TestStatusModelRunOnceCancellationReportsTerminalState`))

**Candidate Revolvr adaptation:** retain the existing settlement-safe run
cancellation and make overlay cancellation a separate focus-owner action.

## Session Header, Replay, and Refresh

**Observed Codex behavior:** session configuration builds session information as
history, initially installs it as the active cell, and removes a transient
header for display modes that should not show it. A clear operation can rebuild
and insert a fresh header after clearing terminal-owned history. ([I29], [I30])

**Observed Codex behavior:** resume replay deliberately routes a safe subset of
persisted turn items through replay-aware handlers and reconstructs completed,
interrupted, or failed turn state without live-only side effects. Initial replay
uses the same bounded retained-line contract as resize rebuilds. ([I31], [I32])

**Current Revolvr behavior:** refresh replaces status projections and preserves
selected identities where possible; the dashboard always redraws a one-line
header rather than committing a one-time session cell. (Revolvr
`internal/tui/model.go:289-315,2189-2199` (`Update`, `headerLines`);
`internal/tui/model_test.go:873-944`
(`TestStatusModelRefreshActionReloadsStatusSnapshot`))

**Accepted Revolvr adaptation:** D6 uses one committed `session-start` cell per
TUI process. It is emitted before bounded canonical replay and contains only
the local `Revolvr` label, the inspected absolute repository root threaded from
`app.StatusResult.ProjectRoot`, and the initial
`app.StatusResult.Initialized` value. Its typed process-local identity is not a
timestamp or rendered string.

Refresh, resize, and overlay open/dismiss do not mutate or re-emit the cell.
Restart creates a new emitted-identity set, emits one new `session-start`, and
then replays the bounded canonical window once. The initialization wording is
explicitly a process-start fact; current command guards use refreshed status
without rewriting terminal history.

TUI-005 fixes the literal process-start wording as `Revolvr`, `Project: ...`,
and `At start: initialized` or `At start: not initialized`. Mutable `Ready` or
`Not initialized` wording belongs to the replaceable live owner, so a refresh
can change current guards without contradicting or duplicating session history.

D6 rejects adding a Revolvr clear action in this overhaul. Clearing terminal-
owned scrollback outside Revolvr is not observed and causes no replay. This is
intentionally smaller than Codex's clear lifecycle and requires no new command,
callback, presentation epoch, or terminal-history owner.

## Intentionally Irrelevant Observations

- Provider/model selection, token usage, attachments, mentions, plugins,
  collaboration, sub-agents, and thread pickers have no Revolvr-overhaul
  contract.
- Codex-specific shell escape, external editor, image, desktop handoff, and
  notification behavior does not imply Revolvr features.
- Codex’s exact command names, colors, glyphs, copy text, and snapshot contents
  are not reusable product specifications.

## Evidence

- **I01** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app.rs:L533-L565 (App)`.
- **I02** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app.rs:L752-L868 (App::handle_tui_event)`.
- **I03** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget/rendering.rs:L9-L92 (ChatWidget::as_renderable)`.
- **I04** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/mod.rs:L1811-L1866 (BottomPane::as_renderable_with_composer_right_reserve)`.
- **I05** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/mod.rs:L560-L644 (BottomPane view-stack ownership)`.
- **I06** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/mod.rs:L646-L703 (BottomPane::handle_key_event)`.
- **I07** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/history_cell/mod.rs:L175-L288 (HistoryCell)`.
- **I08** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget/streaming.rs:L22-L89,L451-L575 (flush_answer_stream; active stream tail)`.
- **I09** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app/agent_message_consolidation.rs:L24-L97 (App::handle_consolidate_agent_message)`.
- **I10** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app/history_ui.rs:L23-L65 (App::insert_history_cell)`.
- **I11** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/insert_history.rs:L59-L112 (history insertion API)`.
- **I12** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/history_cell/mod.rs:L105-L143 (cell modules; HistoryRenderMode)`.
- **I13** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/chat_composer.rs:L2956-L3085 (ChatComposer::prepare_submission_text)`.
- **I14** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/chat_composer.rs:L3428-L3559 (ChatComposer::handle_key_event_without_popup)`.
- **I15** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/chat_composer_history.rs:L315-L351,L374-L465 (recording and navigation)`.
- **I16** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/command_popup.rs:L20-L40,L71-L193 (CommandPopup)`.
- **I17** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget/input_queue.rs:L14-L95 (InputQueueState; PendingInputPreview)`.
- **I18** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget/input_submission.rs:L108-L166,L338-L451 (submission and steer routing)`.
- **I19** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget/interrupts.rs:L16-L123 (InterruptManager)`.
- **I20** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget/tool_requests.rs:L283-L335,L424-L460 (approval/input request routing)`.
- **I21** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/mod.rs:L1491-L1556 (push approval/input views)`.
- **I22** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/request_user_input/mod.rs:L1-L8,L158-L180 (behavior and state)`.
- **I23** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/request_user_input/mod.rs:L201-L215,L269-L279,L871-L883 (composer configuration and progression)`.
- **I24** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/request_user_input/mod.rs:L885-L934 (RequestUserInputOverlay::submit_answers)`.
- **I25** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/request_user_input/mod.rs:L3386-L3423,L3550-L3587,L3625-L3638 (request-input snapshot tests)`.
- **I26** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/approval_overlay.rs:L172-L245,L247-L404 (ApprovalOverlay)`.
- **I27** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/approval_overlay.rs:L500-L535,L579-L618 (cancellation and view contract)`.
- **I28** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/approval_overlay.rs:L1306-L1346,L1496-L1524 (cancellation and stale-resolution tests)`.
- **I29** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget/session_flow.rs:L121-L147 (session info lifecycle)`; `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/history_cell/session.rs:L104-L140,L225-L270 (session cells)`.
- **I30** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app/history_ui.rs:L236-L300 (clear header and terminal UI)`.
- **I31** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget/replay.rs:L1-L14,L20-L77,L80-L175 (thread replay)`.
- **I32** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app/resize_reflow.rs:L1-L15,L124-L178 (replay/reflow contract)`.
