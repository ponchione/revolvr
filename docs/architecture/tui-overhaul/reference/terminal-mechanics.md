# Terminal Mechanics

Evidence pin: `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`.
This document records observable contracts and proof boundaries; it does not
make Codex's terminal backend authoritative or add terminal machinery to
Revolvr. D3 accepts a smaller Bubble Tea hybrid and leaves environment-specific
behavior to the named proofs below.

## Rendering Ownership

**Observed Codex behavior:** normal chat uses an inline terminal viewport so
committed history remains above it in the normal screen buffer. Each draw first
applies viewport changes and flushes pending history, then redraws only the
current frame; the app supplies the chat widget’s desired height and cursor.
([T01], [T02], [T03])

**Observed Codex behavior:** the active cell is flexible and the bottom pane is
fixed. Active content is cleared before repaint and scrolled to its newest rows
when it exceeds its allotted area, preventing stale glyphs after stream or
width changes. ([T04], [T05])

**Current Revolvr behavior:** `RunStatus` starts a normal Bubble Tea program
without `tea.WithAltScreen`; `View` joins a header, one Bubbles viewport, and a
footer into a complete redraw string. (Revolvr `internal/tui/model.go:255-280,746-752`
(`RunStatus`, `View`))

**Accepted Revolvr adaptation:** D3 uses a bounded hybrid. `internal/tui` owns
semantic committed source cells and the live/composer/overlay frame. Bubble Tea
appends each new finalized rendering above the inline program, after which the
normal-screen terminal history owns that row. The current viewport remains
available only for overlay-local scrolling.

## History Insertion and Native Scrollback

**Observed Codex behavior:** finalized history is emitted through terminal
escape operations above the inline viewport rather than ordinary ratatui frame
rendering. The retained `HistoryCell` remains the source; terminal rows are a
presentation cache. ([T06], [T07])

**Observed Codex behavior:** insertion pre-wraps ordinary text, preserves
terminal wrapping for URL-like lines, restores the cursor, and uses either a
partial scroll region or full-screen scrolling. Strategy selection distinguishes
standard terminals, Zellij, and Windows Terminal behavior. ([T07], [T08])

**Observed Codex behavior:** the scrollback tests verify strategy selection and
that full-screen insertion and viewport growth preserve prior terminal history.
([T09])

**Accepted Revolvr adaptation:** use the installed `tea.Println` command, which
queues persistent output above a non-alternate-screen Bubble Tea program. Do
not reproduce Codex's escape implementation, add a terminal backend, or make
rendered rows semantic authority. TUI-010 must prove exact-once composition and
test output before installation; TUI-061 must record plain-terminal and tmux
scroll/copy behavior before support is claimed.

## Resize, Reflow, and Width

**Observed Codex behavior:** retained semantic cells are the source for width
changes. Resize notes width/height changes, debounces a rebuild, discards rows
wrapped for the old size, defers while an overlay owns the surface, clears
Codex-owned terminal history, and reinserts a newly rendered transcript.
([T10], [T11])

**Observed Codex behavior:** a resize during streaming is remembered so final
stream consolidation can force a second rebuild from the finalized source. A
configurable row cap retains the newest rows and prepends a textual path to the
full transcript when older rows are omitted. ([T10], [T12])

**Observed Codex behavior:** cell height uses actual viewport wrapping; active
stream width subtracts wrapper columns and bottoms out at one usable column.
The session header and typed-question snapshots include narrow, wrapping, and
truncation cases. ([T13], [T14], [T15])

**Current Revolvr behavior:** `tea.WindowSizeMsg` updates stored geometry,
resizes the single viewport, and regenerates wrapped presentation strings.
Content width never falls below one; widths below the compact threshold use
the compact presentation. (Revolvr `internal/tui/model.go:287-294,2115-2150`
(`Update`, `resizeViewport`, `formatContent`))

**Current Revolvr behavior:** 100-column and 40-column render tests assert exact
dashboard rows and a maximum line width; a 40x24 test bounds dashboard chrome
and verifies composer open/close. (Revolvr
`internal/tui/model_test.go:2509-2648`
(`TestStatusModelWideRenderSnapshot`, `TestStatusModelNarrowRenderSnapshot`,
`TestStatusModelDashboardChromeAndComposer`))

**Accepted Revolvr adaptation:** D3 deliberately splits reflow ownership.
`internal/tui` redraws retained source, the live cell, composer, and overlay at
the new width without re-emitting a committed identity. Previously appended
rows remain terminal-owned and receive only the environment's native soft
reflow. Revolvr never clears or reinserts them. TUI-011/TUI-060 prove the
managed frame and no-replay invariant; TUI-061 records native behavior and any
unsupported environment.

Accepted D6 applies the same rule to `session-start`: it is emitted once before
bounded history for each process, never on refresh, resize, or overlay
transitions, and once again only when a new process starts. No Revolvr clear
action or terminal-history clearing mechanism is added.

TUI-005 accepts 80x24 as normal geometry and 40x24 as the minimum supported
geometry. Its [source snapshots](../README.md#accepted-experience-state-snapshots)
keep required state, safety, cancellation, outcome, focus, and next-action text
at 40 columns. Below 40 columns is intentional best effort: required text wraps,
secondary live detail may compact to `…`, overlays scroll, and committed rows
are never cleared or replayed.

## Terminal Lifecycle and Restoration

**Observed Codex behavior:** initialization requires TTY stdin/stdout, enables
bracketed paste and raw mode, attempts enhanced keyboard/focus modes, installs
an initialization guard, probes terminal capabilities, and constructs an inline
terminal. ([T16], [T17])

**Observed Codex behavior:** restoration attempts every owned cleanup step while
preserving the first error: keyboard reporting, bracketed paste, focus events,
raw mode, cursor style, cursor visibility, and stderr handling on final exit.
([T18])

**Observed Codex behavior:** alternate screen is reserved for overlays that need
it; entering saves the inline viewport and leaving restores it. Ctrl-Z exits any
alternate screen, restores terminal/stderr state before `SIGTSTP`, reapplies
modes on resume, probes cursor position, and later realigns the inline viewport
or restores the overlay. ([T19], [T20])

**Accepted Revolvr adaptation:** rely on Bubble Tea for program lifecycle and
restoration. `StatusModel` retains only the existing rule that cancellation and
quit wait for the matching operation result before final emission and
`tea.Quit`. Add no custom restoration code unless TUI-062 proves a focused gap.

## Styling and Text Accessibility

**Observed Codex behavior:** the style guide uses bold headers, terminal-default
primary text, dim secondary text, ANSI cyan for selection/status, green for
success/addition, and red for failure/deletion. It rejects custom foregrounds
and black/white because terminal themes own contrast. ([T21])

**Observed Codex behavior:** terminal startup probes default colors and caches
color capability. Rich transcript rendering has a separate raw mode that emits
copy-friendly plain lines, and a light-palette composer snapshot checks a light
terminal case. ([T17], [T22], [T23])

**Current Revolvr behavior:** the TUI already uses default, bold, dim, cyan,
green, and red semantic roles, while geometry tests normalize ANSI before
asserting textual rows. (Revolvr `internal/tui/model.go:44-52,4052-4105`
(`styleHeaderLines`, `styleFooterLines`, `styleContentLine`);
`internal/tui/model_test.go:2663-2675`
(`normalizedViewLines`))

**Candidate Revolvr adaptation:** keep state and actions legible from words and
symbols after ANSI removal; use color only as redundant emphasis.

### Known Limits

- **Observed Codex behavior:** raw transcript mode is a copy-friendly rendering
  mode, not proof that every interactive surface has a global styles-off mode.
  ([T22])
- **Open product decision:** this study found no single pinned end-to-end
  no-color snapshot for the complete shell. TUI-063 must prove Revolvr’s actual
  `NO_COLOR`/non-color behavior rather than infer it from Codex.
- **Open product decision:** terminal probes and virtual-terminal behavior differ
  by platform; this source study did not run Codex against Revolvr’s supported
  terminal matrix.

## Bubble Tea Boundary

Revolvr pins Bubble Tea `v1.3.4`, Bubbles `v0.20.0`, and Lip Gloss `v1.1.0`.
(Revolvr `go.mod:6-8`.)

| Concern | Installed Bubble Tea boundary | Revolvr proof still required |
|---|---|---|
| Event loop and complete-frame renderer | `tea.NewProgram(...).Run()` already owns input, rendering, signals, and terminal teardown for the current program; Revolvr invokes that path at `internal/tui/model.go:255-280` | No new proof for unchanged behavior; TUI-013 must show the target shell still exits cleanly |
| Resize delivery | Bubble Tea delivers `tea.WindowSizeMsg`; Revolvr handles it at `internal/tui/model.go:287-294` | TUI-011/TUI-060 must prove semantic-cell reflow and settlement after the ownership change |
| Inline versus alternate screen | Alternate screen is opt-in; Revolvr does not pass `tea.WithAltScreen` at `internal/tui/model.go:255-280`; `tea.Println` persists output above a normal-screen program | TUI-010 must prove exact-once append composition and test output; TUI-061 must establish real scrollback behavior |
| Viewport scrolling | Bubbles viewport already powers current focused-view scrolling at `internal/tui/model.go:97,725-743` | Accepted D3 limits it to overlay-local scrolling; it does not own committed history |
| Suspend/resume and failures | Bubble Tea provides its own program lifecycle for the installed version | TUI-062 must exercise Ctrl-Z, resume, normal exit, cancellation settlement, and injected error paths in Revolvr; no custom terminal layer should be assumed necessary first |

## Defining Tests and Snapshots

| Contract | Pinned proof |
|---|---|
| Full-screen insertion preserves scrollback | `full_screen_history_insertion_preserves_terminal_scrollback` and `full_screen_viewport_growth_preserves_terminal_scrollback` ([T09]) |
| Reflow retains more than the visible viewport and honors caps | `resize_reflow_preserves_configured_scrollback_beyond_the_visible_viewport`, `initial_resume_replay_retains_scrollback_beyond_the_visible_viewport`, and capped/narrow notice tests ([T12]) |
| Active transcript does not leave stale glyphs | `HistoryCell` render clears the active area before painting ([T05]); target Revolvr still needs a settlement snapshot |
| Composer geometry and palette | `empty`, `large`, `light_terminal_palette_composer`, and footer-mode snapshots are generated through the common composer snapshot helper ([T23], [T24]) |
| Session header narrow width | `session_header_clamps_to_narrow_width.snap`, identified by its test/snapshot path ([T15]) |
| Typed question width behavior | `request_user_input_tight_height`, `request_user_input_wrapped_options`, `request_user_input_long_option_text`, and `request_user_input_footer_wrap` ([T15]) |
| Transcript overlay includes live state | `transcript_overlay_renders_live_tail.snap` and `transcript_overlay_completed_stream.snap`; these are proof identities only ([T25]) |

## Evidence

- **T01** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui.rs:L421-L429 (tui::init inline contract)`.
- **T02** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui.rs:L973-L1049 (Tui::draw)`.
- **T03** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app.rs:L881-L910 (App::render_chat_widget_frame)`.
- **T04** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget/rendering.rs:L21-L92 (ChatWidget::as_renderable)`.
- **T05** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/history_cell/mod.rs:L290-L306 (HistoryCell render)`.
- **T06** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/insert_history.rs:L1-L5 (module contract)`; `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app/history_ui.rs:L23-L65 (App::insert_history_cell)`.
- **T07** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/insert_history.rs:L106-L145,L165-L227,L230-L267 (insertion and wrapping)`.
- **T08** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui/scrollback.rs:L17-L79 (ScrollbackStrategy)`.
- **T09** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui/scrollback_tests.rs:L18-L50,L53-L169 (strategy and preservation tests)`.
- **T10** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app/resize_reflow.rs:L1-L15,L344-L377,L392-L458 (resize/reflow lifecycle)`.
- **T11** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app/resize_reflow.rs:L461-L510 (App::reflow_transcript_now)`.
- **T12** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app/resize_reflow_tests.rs:L25-L97,L99-L208 (reflow/replay/cap tests)`; `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/app/resize_reflow.rs:L569-L672 (cap and notice)`.
- **T13** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/history_cell/mod.rs:L175-L230 (HistoryCell width measurement)`.
- **T14** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/chatwidget.rs:L1605-L1618 (ChatWidget::current_stream_width)`.
- **T15** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/history_cell/snapshots/codex_tui__history_cell__tests__session_header_clamps_to_narrow_width.snap:L1-L5 (snapshot identity)`; `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/request_user_input/mod.rs:L3386-L3437,L3550-L3587,L3625-L3638 (width snapshot tests)`.
- **T16** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui.rs:L227-L247 (set_modes)`.
- **T17** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui.rs:L421-L513 (tui::init)`.
- **T18** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui.rs:L292-L375 (restore_common; restore_after_exit)`.
- **T19** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui.rs:L829-L865 (enter/leave alternate screen)`.
- **T20** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui/job_control.rs:L25-L98,L199-L209 (SuspendContext; suspend_process)`.
- **T21** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/styles.md:L1-L21 (style guide)`.
- **T22** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/history_cell/mod.rs:L139-L145,L184-L210 (rich/raw rendering)`; `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/tui.rs:L637-L640 (color cache)`.
- **T23** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/chat_composer.rs:L5093-L5113 (light_terminal_palette_composer)`.
- **T24** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/bottom_pane/chat_composer.rs:L5248-L5299 (composer snapshot helper and footer snapshots)`.
- **T25** — `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/snapshots/codex_tui__pager_overlay__tests__transcript_overlay_renders_live_tail.snap:L1-L5 (snapshot identity)`; `Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 codex-rs/tui/src/snapshots/codex_tui__pager_overlay__tests__transcript_overlay_completed_stream.snap:L1-L5 (snapshot identity)`.
