# Revolvr Mapping

Codex evidence is pinned to
`8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`; its cited contracts live in the
[interaction model](interaction-model.md) and
[terminal mechanics](terminal-mechanics.md). D2, D3, and D5 are accepted; this
mapping does not resolve D4 or D6.

## Current Boundary

**Current Revolvr behavior:** one `StatusModel` owns view selection, one Bubbles
viewport, command-composer state, live-run state, typed-answer state, and
application callbacks. The CLI injects application functions; the TUI does not
own ledger or workflow persistence. ([R01], [R02])

**Current Revolvr behavior:** the dashboard and run detail consume the
application’s semantic `RunTimeline` projection, while focused views consume
canonical run or autonomous projections. ([R03], [R04])

**Accepted D3 boundary:** keep that application-service seam. The smallest
shell change is inside `internal/tui.StatusModel`: retain semantic source and
emitted identities, append finalized renderings through installed
`tea.Println`, and leave emitted rows to normal-screen terminal history.
Introduce no new service, dependency, terminal backend, or escape layer unless
the bounded proof fails and the decision is explicitly reopened.

## Contract Mapping

Classification is about the smallest next boundary, not completion status of
the overhaul.

| Referenced contract | Current Revolvr evidence | Classification | Smallest likely seam | Task / decision |
|---|---|---|---|---|
| [Committed transcript + replaceable live region](interaction-model.md#shell-composition-and-focus) | One viewport redraw contains all content and chrome. ([R01], [R05]) | `accepted; proof required` | `StatusModel` source/emitted identity plus `tea.Println` and managed-frame composition | [TUI-010](../tasks/tui-010-prove-shell-composition.md) / accepted D3 |
| [Source-backed resize/reflow](terminal-mechanics.md#resize-reflow-and-width) | Window size regenerates wrapped viewport strings; no retained cell source exists. ([R05]) | `accepted; proof required` | `Update(tea.WindowSizeMsg)` reflows only retained/managed state and never re-emits history | [TUI-011](../tasks/tui-011-prove-resize-reflow.md) / accepted D3 |
| [Live-to-committed settlement](interaction-model.md#live-to-committed-settlement) | Run state is mutable and historical status is refreshed afterward, but there is no single cell-settlement boundary. ([R06]) | `accepted; proof required` | existing run completion handlers plus stable identity and one-time append | [TUI-012](../tasks/tui-012-prove-active-settlement.md) / accepted D3 |
| Proven shell installation | The current program is already one Bubble Tea model without alternate-screen opt-in. ([R01]) | `accepted after proof` | `RunStatus` program options and proven `StatusModel` hybrid | [TUI-013](../tasks/tui-013-install-terminal-shell.md) / accepted D3 |
| [Semantic cell interface](interaction-model.md#semantic-cell-categories) | Semantic timeline rows and focused projections exist, but presentation is assembled as strings. ([R03], [R04]) | `presentation change` | small TUI-owned cell value/render functions over existing projections | [TUI-020](../tasks/tui-020-define-transcript-cells.md) / D6 only for a session cell |
| [Replay from durable semantic source](interaction-model.md#session-header-replay-and-refresh) | `app.RunTimeline` deterministically projects ledger history and tests completed/failure/fallback cases. ([R03]) | `presentation change` | replay a bounded source window once at startup and append only new stable identities on refresh | [TUI-021](../tasks/tui-021-project-historical-runs.md) / accepted D3 |
| Live/history identity reconciliation | Refresh preserves selection and focused refresh reloads canonical history. ([R07]) | `proof required` | stable ledger event/run identity plus process-local emitted set | [TUI-022](../tasks/tui-022-reconcile-live-history.md) / accepted D3 |
| [Normally focused composer](interaction-model.md#composer-ownership-and-submission) | Composer opens only after `/` and closes after submission or escape. ([R08]) | `presentation change` | `commandComposerState`, top-level key routing, footer | [TUI-030](../tasks/tui-030-make-composer-primary.md) / accepted D2 |
| Plain-text submission | `submitCommand` treats the first field as a slash command; task entry already holds an editable draft and calls reviewed `AddTaskAndCommit` only on Enter. ([R02], [R08], [R17]) | `presentation change` | transfer initialized idle text into existing `taskEntryState`; reject and preserve it otherwise | [TUI-031](../tasks/tui-031-implement-plain-text-input.md) / accepted D2/D5; no app prerequisite |
| [Contextual command discovery and history](interaction-model.md#commands-and-history) | `/` plus enter opens static help; no filtered popup or submission history exists. ([R08]) | `presentation change` | composer-local filtered list over the existing `submitCommand` vocabulary | [TUI-032](../tasks/tui-032-add-contextual-command-discovery.md) / D2 |
| One replaceable live operation | `runOnceState` already holds one active operation and rejects overlapping starts. ([R06], [R09]) | `presentation change` | render existing `runOnceState` only in the managed live frame | [TUI-040](../tasks/tui-040-render-live-operation.md) / accepted D3 |
| [Queued input with explicit categories](interaction-model.md#queued-input-and-interruption) | No application callback or domain projection owns queued composer input. Active-run key routing rejects conflicting actions. ([R02], [R06]) | `skip` | none; accepted D5 rejects steering and queued/deferred messages | No task; TUI-041 removed |
| [Overlay focus stack](interaction-model.md#overlays-and-focus-transfer) | Focused views replace the current enum view and remember one source; there is no shared stacked overlay shell. ([R10]) | `proof required` | replace `openFocusedView`/`closeFocusedView` ownership with one overlay state | [TUI-050](../tasks/tui-050-add-overlay-shell.md) / D4 |
| Tasks presentation in overlay | Tasks already render and navigate in the shared viewport. ([R11]) | `presentation change` | route existing Tasks render/key paths through the accepted overlay shell | [TUI-051](../tasks/tui-051-move-tasks-overlay.md) / D4 |
| Runs and Run Detail in overlay | Both views already use canonical status/history callbacks. ([R02], [R11]) | `presentation change` | retain renderers and move only view/focus ownership | [TUI-052](../tasks/tui-052-move-runs-overlay.md) / D4 |
| Preflight in overlay | Preflight is callback-backed and guarded against active-run overlap. ([R02], [R06]) | `presentation change` | retain `preflightState` and callback; move presentation ownership | [TUI-053](../tasks/tui-053-move-preflight-overlay.md) / D4 |
| Workflow in overlay | Autonomous selectors/projection and scrolling already live in `StatusModel`. ([R02], [R12]) | `presentation change` | retain `autonomousState`; move its render/key route | [TUI-054](../tasks/tui-054-move-workflow-overlay.md) / D4 |
| Change Summary in overlay | Focused diff already renders canonical artifacts and returns to its source at narrow width. ([R04], [R10]) | `presentation change` | `renderFocusedDiff` under the shared overlay shell | [TUI-055](../tasks/tui-055-move-change-summary-overlay.md) / D4 |
| Evidence in overlay | Focused evidence already reloads canonical history on refresh. ([R04], [R07]) | `presentation change` | `renderFocusedEvidence` under the shared overlay shell | [TUI-056](../tasks/tui-056-move-evidence-overlay.md) / D4 |
| [Typed approval overlay](interaction-model.md#approvals) | Approval is a focused projection; accepted answers pass typed task/question/revision/hash/option identity to the app callback. ([R04], [R13]) | `presentation change` | keep `autonomousAnswerState` and `AnswerInput`; change only focus/render owner | [TUI-057](../tasks/tui-057-move-approval-overlay.md) / D4 |
| [Typed question overlay](interaction-model.md#typed-questions) | Needs-input requires a current typed question, explicit option, double confirmation, and stale-question rejection. ([R13]) | `presentation change` | retain the existing answer state machine and expose it through the shared overlay; no free-form route | [TUI-058](../tasks/tui-058-move-needs-input-overlay.md) / accepted D2, open D4 |
| [Width/geometry proof](terminal-mechanics.md#resize-reflow-and-width) | Exact 100-column, 40-column, and 40x24 render tests already bound lines and chrome for the current dashboard. ([R14]) | `proof required` | prove managed-frame reflow and no committed-identity replay | [TUI-060](../tasks/tui-060-lock-geometry-snapshots.md) / accepted D3, open D6 |
| [Terminal-native scrollback](terminal-mechanics.md#history-insertion-and-native-scrollback) | Current code uses one Bubbles viewport and has no project-owned insertion seam. ([R01], [R05]) | `accepted; environment proof required` | installed `tea.Println` plus plain-terminal/tmux matrix; no production escape layer | [TUI-061](../tasks/tui-061-verify-terminal-scrollback.md) / accepted D3 |
| [Suspend/restore/error lifecycle](terminal-mechanics.md#terminal-lifecycle-and-restoration) | Lifecycle is delegated to `tea.NewProgram`; active quit waits for run settlement in model tests. ([R01], [R09]) | `accepted boundary; proof required` | Bubble Tea lifecycle plus existing model settlement | [TUI-062](../tasks/tui-062-verify-terminal-lifecycle.md) / accepted D3 |
| [Text-first semantic styling](terminal-mechanics.md#styling-and-text-accessibility) | Default text plus bold/dim/cyan/green/red roles exist, and tests strip ANSI before textual assertions. ([R15], [R14]) | `proof required` | existing style functions and styles-disabled test environment | [TUI-063](../tasks/tui-063-verify-text-accessibility.md) / none |
| Remove dashboard-only presentation | Current dashboard still owns header/status/timeline/footer assembly. ([R05], [R16]) | `presentation change` | delete obsolete dashboard render/chrome and viewport history ownership only after parity | [TUI-070](../tasks/tui-070-remove-dashboard-presentation.md) / accepted D3, open D6 |
| Operator documentation | No useful Codex analog; Revolvr commands and accepted decisions are the authority. | `skip` | current Revolvr docs after behavior lands | [TUI-071](../tasks/tui-071-update-operator-docs.md) / D2–D6 as accepted |
| Overhaul acceptance | No useful Codex analog; closure depends on Revolvr tests and terminal records. | `proof required` | existing task acceptance record | [TUI-072](../tasks/tui-072-close-overhaul-acceptance.md) / D2–D6 as accepted |

## Decision Status

- **Accepted D2:** initialized idle plain text creates only an ephemeral,
  editable task draft for the existing reviewed Add Task flow. All other plain
  text is rejected, and typed needs-input remains option-only.
- **Accepted D5:** active steering and queued/deferred operator messages are
  unavailable. The autonomous task queue is not a message queue.
- **Accepted D3:** `internal/tui` owns bounded semantic source cells, stable
  settlement identity, and the live/composer/overlay frame. Bubble Tea appends
  finalized renderings once to terminal-owned normal-screen history. The TUI
  never clears or re-emits committed rows on resize; a viewport is overlay-only.
- **Open product decision D4:** overlay migration order and parity gate.
- **Open product decision D6:** whether a one-time session header is committed,
  refreshed, or omitted.

## Prioritized Evidence Gaps

1. Accepted D3 still lacks the minimal Go/Bubble Tea proof showing
   `tea.Println` commitment, replaceable live content, test output, managed
   resize, and settlement without duplicated rows.
2. D3/TUI-061 still lacks real-terminal evidence for the supported terminal and
   multiplexer matrix; unit-render strings cannot establish native scrollback.
3. TUI-062 lacks recorded Ctrl-Z/resume, normal exit, cancellation-settlement,
   and injected-error restoration checks for the target shell.
4. TUI-063 lacks a complete styles-disabled target snapshot; ANSI-normalized
   current tests establish textual content but not final focus/status clarity.
5. D6 lacks an accepted session-header lifecycle and replay/refresh snapshot.
6. D4 lacks per-view overlay parity evidence, especially focus return and
   narrow-height behavior for typed answers.

These are evidence gaps for existing decisions and tasks, not a new backlog.
TUI-003 is the next bounded product-decision task.

## Revolvr Evidence

- **R01** — `Revolvr internal/tui/model.go:L77-L98,L232-L280 (StatusModel; RunStatus)`.
- **R02** — `Revolvr internal/tui/model.go:L100-L147 (callback types; StatusActions; RunOptions)`; `Revolvr internal/cli/root.go:L1681-L1727 (newTUICommand injection seam)`.
- **R03** — `Revolvr internal/app/timeline.go:L14-L84,L86-L153 (RunTimeline)`; `Revolvr internal/app/timeline_test.go:L12-L73 (TestRunTimelineCompletedRun)`.
- **R04** — `Revolvr internal/tui/model.go:L2449-L2683 (focused change, evidence, and approval renderers)`; `Revolvr internal/tui/architecture_024_test.go:L16-L120 (narrow navigation and focused refresh tests)`.
- **R05** — `Revolvr internal/tui/model.go:L287-L294,L746-L752,L2017-L2024,L2115-L2150 (resize and viewport redraw)`.
- **R06** — `Revolvr internal/tui/model.go:L1198-L1370,L1501-L1577 (run state, admission, active keys, cancellation)`.
- **R07** — `Revolvr internal/tui/model.go:L294-L315 (refresh identity handling)`; `Revolvr internal/tui/architecture_024_test.go:L86-L120 (TestFocusedRunRefreshReloadsCanonicalHistory)`.
- **R08** — `Revolvr internal/tui/model.go:L195-L198,L515-L528,L1845-L2014 (composer activation and command dispatch)`; `Revolvr internal/tui/model_test.go:L2602-L2648 (TestStatusModelDashboardChromeAndComposer)`.
- **R09** — `Revolvr internal/tui/model_test.go:L1234-L1448 (run cancellation and settlement tests)`.
- **R10** — `Revolvr internal/tui/model.go:L2038-L2062 (focused-view ownership)`; `Revolvr internal/tui/architecture_024_test.go:L66-L83 (focus navigation and return)`.
- **R11** — `Revolvr internal/tui/model.go:L2153-L2187,L2318-L2439 (view dispatch; Tasks/Runs/Run Detail)`.
- **R12** — `Revolvr internal/tui/model.go:L959-L1197,L2714-L2908 (autonomous projection, input, and rendering)`; `Revolvr internal/tui/autonomous_test.go:L75-L99 (plain narrow scrollable lifecycle test)`.
- **R13** — `Revolvr internal/tui/model.go:L1050-L1128 (typed answer state machine)`; `Revolvr internal/tui/autonomous_test.go:L101-L181 (explicit selection and stale-question tests)`; `Revolvr internal/tui/architecture_024_test.go:L122-L188 (approval answer integration)`.
- **R14** — `Revolvr internal/tui/model_test.go:L2509-L2648,L2663-L2680 (wide/narrow/chrome snapshots and ANSI-normalized width checks)`.
- **R15** — `Revolvr internal/tui/model.go:L44-L52,L4052-L4105 (style roles and semantic application)`.
- **R16** — `Revolvr internal/tui/model.go:L2189-L2290,L2292-L2315,L3319-L3407 (dashboard chrome and timeline presentation)`.
- **R17** — `Revolvr internal/tui/model.go:L1790-L1843,L2064-L2112 (reviewed task-entry state)`; `Revolvr internal/cli/root.go:L1706-L1708 (AddTask callback)`; `Revolvr internal/app/task_commit.go:L15-L89 (AddTaskAndCommit)`.
