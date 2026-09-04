# Codex TUI Startup and Terminal Runtime Baseline

- Status: Research note; no UI behavior implemented
- Date: 2026-09-04
- Installed visual reference: `codex-cli 0.153.2`
- Local source reference: `.reference/codex` at
  `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`

The installed binary and pinned source checkout are separate anchors: the
binary defines the currently observed visual behavior, while the checkout
provides inspectable terminal mechanics. This note does not assert that the
checkout is the exact release source for `0.153.2`.

## Executive summary

**Observed.** Running bare `codex` is intentionally the interactive path, not
a command that prints a status page: Clap flattens the TUI arguments into the
root parser, and absent a subcommand the dispatcher calls the interactive TUI.
`codex exec` and `codex review` instead call the non-interactive executor
([`cli/src/main.rs:100-139`](../../.reference/codex/codex-rs/cli/src/main.rs#L100-L139),
[`cli/src/main.rs:1090-1179`](../../.reference/codex/codex-rs/cli/src/main.rs#L1090-L1179)).

Codex becomes app-like because it takes explicit ownership of a TTY, terminal
modes, cursor, viewport, event streams, and restoration. It uses Ratatui over a
Crossterm backend, but wraps Ratatui's terminal with a custom two-buffer,
viewport-aware renderer that writes only buffer diffs. Its normal shell is an
inline viewport that preserves scrollback; it can temporarily expand into the
alternate screen for full-screen surfaces, despite alternate-screen use being
enabled by default in configuration
([`tui/src/tui.rs:421-513`](../../.reference/codex/codex-rs/tui/src/tui.rs#L421-L513),
[`tui/src/custom_terminal.rs:126-169`](../../.reference/codex/codex-rs/tui/src/custom_terminal.rs#L126-L169),
[`tui/src/custom_terminal.rs:284-343`](../../.reference/codex/codex-rs/tui/src/custom_terminal.rs#L284-L343),
[`tui/src/lib.rs:1795-1807`](../../.reference/codex/codex-rs/tui/src/lib.rs#L1795-L1807)).

Codex also renders before slow startup finishes: it creates a real, editable
but non-submitting composer, pumps terminal events alongside bootstrap futures,
and later transfers the same terminal and draft into the main application
([`tui/src/startup_draft.rs:81-164`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L81-L164),
[`tui/src/startup_draft.rs:202-224`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L202-L224)).

**Gap.** Revolvr's bare command still prints Cobra help; the TUI is a `tui`
subcommand that synchronously loads status before starting Bubble Tea. Bubble
Tea owns its terminal loop, while Revolvr's model emits session/history cells
to scrollback on `Init` and renders one assembled string. There is no explicit
Revolvr-owned TTY gate, startup draft, viewport policy, alternate-screen
policy, terminal probing, stderr quarantine, or panic restoration layer
([`internal/cli/root.go:84-134`](../../internal/cli/root.go#L84-L134),
[`internal/cli/root.go:1681-1713`](../../internal/cli/root.go#L1681-L1713),
[`internal/tui/model.go:465-535`](../../internal/tui/model.go#L465-L535),
[`internal/tui/model.go:951-960`](../../internal/tui/model.go#L951-L960)).

**Recommendation.** Prioritize the launch contract and first 80x24 frame before
feature parity: bare-command dispatch, immediate shell ownership, one stable
initial composition, then lifecycle hardening and only afterward transcript
settlement and overlays. Keep the accepted boundary: reimplement in Go/Bubble
Tea; do not copy, vendor, port, or embed Codex.

Do not begin by enabling the alternate screen or replacing Bubble Tea. A live
PTY trace showed that ordinary Codex launch is inline, and Bubble Tea already
provides raw mode, an inline repainting renderer, bracketed paste, signal and
panic cleanup, optional alternate screen, resize events, and terminal
release/restore. The first evidenced Revolvr failures occur above that layer:
dispatch, synchronous pre-render loading, and startup history emission.

## Empirical 80x24 launch trace (observed)

Both installed applications were launched in a fresh 80x24 pseudo-terminal
with `script`; each was allowed three seconds to initialize and then exited
with Ctrl-C. Escape-sequence counts below describe that one run, not a stable
performance target.

- `codex-cli 0.153.2` immediately painted its bordered session header with
  `model: loading` and `directory: loading`, plus the composer and shortcut
  footer. It replaced those placeholders as startup resolved. The run emitted
  54 paired synchronized-update sequences (`CSI ? 2026 h/l`), one paired
  bracketed-paste transition, one paired focus-reporting transition, and no
  alternate-screen transition (`CSI ? 1049 h/l`). This directly confirms that
  the ordinary app-like launch is an inline, repeatedly updated surface—not a
  full-screen alternate-buffer switch.
- `go run ./cmd/revolvr tui` emitted no synchronized-update or alternate-screen
  sequences. Bubble Tea paired bracketed-paste and cursor hide/show on normal
  Ctrl-C exit, but the first visible content was the session cell followed by
  the latest run's historical timeline and result before the composer. The
  perceived “dump” is therefore visible content selected by Revolvr and
  published through `tea.Println`, not evidence that Bubble Tea failed to enter
  terminal mode.

The Codex trace also encountered a real MCP startup warning without losing the
composer. This reinforces the source-level finding that startup progress and
failures are rendered inside the owned shell rather than printed before it.

## Codex startup call path (observed)

1. `MultitoolCli` declares the interactive `codex_tui::Cli` as flattened root
   arguments and an optional subcommand. Therefore `codex [OPTIONS] [PROMPT]`
   is the primary parser shape, rather than `codex tui`
   ([`cli/src/main.rs:100-129`](../../.reference/codex/codex-rs/cli/src/main.rs#L100-L129)).
2. The top-level match sends `None` (and the special `agents` route) to
   `run_interactive_tui`; `exec` and `review` are separate executor branches
   ([`cli/src/main.rs:1090-1179`](../../.reference/codex/codex-rs/cli/src/main.rs#L1090-L1179)).
3. Interactive startup rejects or confirms a `TERM=dumb` environment and has
   explicit TTY checks for the local agents overview before resolving a remote
   endpoint and calling `codex_tui::run_main` through a recovery wrapper
   ([`cli/src/main.rs:2570-2617`](../../.reference/codex/codex-rs/cli/src/main.rs#L2570-L2617)).
4. `codex_tui::run_main` delegates to startup orchestration and converts an
   intentional startup cancellation into a normal user-requested exit
   ([`tui/src/lib.rs:929-960`](../../.reference/codex/codex-rs/tui/src/lib.rs#L929-L960)).
5. Orchestration chooses the first protected surface: session picker for
   resume/fork/agents, onboarding when first-login preflight requires it, and
   otherwise the composer. It then creates `StartupDraft` before daemon and
   configuration bootstrap work
   ([`tui/src/startup_orchestration.rs:138-180`](../../.reference/codex/codex-rs/tui/src/startup_orchestration.rs#L138-L180)).
6. `StartupDraft::new` initializes and clears the terminal, creates the `Tui`,
   event channel, session header, and bottom pane, and draws the initial screen
   when that screen is the composer
   ([`tui/src/startup_draft.rs:99-135`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L99-L135),
   [`tui/src/startup_draft.rs:270-282`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L270-L282)).
7. Once configuration, server bootstrap, onboarding/trust, and startup review
   work complete, `run_ratatui_app` selects the alternate-screen policy and
   hands the same `Tui` plus startup draft to `App::run`
   ([`tui/src/lib.rs:1600-1645`](../../.reference/codex/codex-rs/tui/src/lib.rs#L1600-L1645),
   [`tui/src/lib.rs:1719-1748`](../../.reference/codex/codex-rs/tui/src/lib.rs#L1719-L1748)).

## Screen-writing and runtime model (observed)

### Terminal mechanisms and libraries

- `codex-tui` directly depends on Crossterm with bracketed-paste and event-stream
  features, Ratatui, Tokio Stream, and Futures
  ([`tui/Cargo.toml:73-90`](../../.reference/codex/codex-rs/tui/Cargo.toml#L73-L90),
  [`tui/Cargo.toml:112-112`](../../.reference/codex/codex-rs/tui/Cargo.toml#L112-L112),
  [`tui/Cargo.toml:158-158`](../../.reference/codex/codex-rs/tui/Cargo.toml#L158-L158)).
- Initialization requires both stdin and stdout TTYs; it enables virtual
  terminal processing, bracketed paste, raw mode, keyboard enhancement, and
  focus events (focus events are disabled on Windows). It probes cursor
  position, default colors, and enhanced-key support, creates a
  `CrosstermBackend<Stdout>`, and installs a stderr guard
  ([`tui/src/tui.rs:227-247`](../../.reference/codex/codex-rs/tui/src/tui.rs#L227-L247),
  [`tui/src/tui.rs:421-513`](../../.reference/codex/codex-rs/tui/src/tui.rs#L421-L513)).
- The custom terminal stores current/previous Ratatui buffers, viewport bounds,
  screen size, cursor position, and visible history count. Each flush computes
  buffer differences and sends only resulting draw commands to the backend
  ([`tui/src/custom_terminal.rs:126-150`](../../.reference/codex/codex-rs/tui/src/custom_terminal.rs#L126-L150),
  [`tui/src/custom_terminal.rs:284-293`](../../.reference/codex/codex-rs/tui/src/custom_terminal.rs#L284-L293)).
- Every frame is fully rendered into a buffer, compared to the preceding frame,
  and reduced to terminal updates; explicit invalidation forces spaces to
  repaint after out-of-band terminal operations
  ([`tui/src/custom_terminal.rs:321-343`](../../.reference/codex/codex-rs/tui/src/custom_terminal.rs#L321-L343),
  [`tui/src/custom_terminal.rs:484-510`](../../.reference/codex/codex-rs/tui/src/custom_terminal.rs#L484-L510)).

### Inline viewport, history, and alternate screen

Codex initializes in inline mode at the probed cursor position, not by blindly
clearing and repainting the entire terminal. `Tui::draw` sizes a viewport to
the renderable's desired height, grows/scrolls it when necessary, clears stale
cells when its bounds move, flushes queued history above it, and executes the
whole update through Crossterm's synchronized-update facility
([`tui/src/tui.rs:421-505`](../../.reference/codex/codex-rs/tui/src/tui.rs#L421-L505),
[`tui/src/tui.rs:948-1048`](../../.reference/codex/codex-rs/tui/src/tui.rs#L948-L1048)).
This is why committed conversation can remain normal terminal scrollback while
the active composer/status area is repeatedly repainted rather than dumped.

Alternate screen is a reversible overlay mechanism: entering emits
`EnterAlternateScreen` and DEC private mode `?1007h` for alternate scroll,
saves the inline viewport, expands to terminal size, and clears; leaving emits
the inverse sequences and restores the saved viewport
([`tui/src/tui.rs:250-290`](../../.reference/codex/codex-rs/tui/src/tui.rs#L250-L290),
[`tui/src/tui.rs:829-865`](../../.reference/codex/codex-rs/tui/src/tui.rs#L829-L865)).
`--no-alt-screen` forces inline-only operation; otherwise `always` and default
`auto` enable alternate screen while `never` disables it
([`tui/src/cli.rs:72-76`](../../.reference/codex/codex-rs/tui/src/cli.rs#L72-L76),
[`tui/src/lib.rs:1795-1807`](../../.reference/codex/codex-rs/tui/src/lib.rs#L1795-L1807)).

### Event and render loop

`Tui` combines a Crossterm-backed event broker with a broadcast draw channel;
frame requesters schedule draws without requiring every producer to write the
terminal directly
([`tui/src/tui.rs:585-663`](../../.reference/codex/codex-rs/tui/src/tui.rs#L585-L663),
[`tui/src/tui.rs:811-827`](../../.reference/codex/codex-rs/tui/src/tui.rs#L811-L827)).
During early startup, `tokio::select!` polls the startup future and that event
stream together, preserving edits while disallowing submission
([`tui/src/startup_draft.rs:202-224`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L202-L224),
[`tui/src/startup_draft.rs:324-370`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L324-L370)).
The main loop selects among internal app events, active-thread events, terminal
events, app-server events, and timers. Draw/resize/resume/focus events render
the chat widget at its desired height and place the cursor through the frame
API
([`tui/src/app/startup.rs:709-825`](../../.reference/codex/codex-rs/tui/src/app/startup.rs#L709-L825),
[`tui/src/app.rs:812-868`](../../.reference/codex/codex-rs/tui/src/app.rs#L812-L868),
[`tui/src/app.rs:881-912`](../../.reference/codex/codex-rs/tui/src/app.rs#L881-L912)).

### Teardown and panic restoration

Restoration resets keyboard reporting, disables bracketed paste and focus
events, disables raw mode, restores Windows input state where applicable,
restores the default cursor shape, shows the cursor, and finishes stderr
suppression
([`tui/src/tui.rs:304-375`](../../.reference/codex/codex-rs/tui/src/tui.rs#L304-L375)).
There are layered guarantees: an initialization guard protects partial setup;
a panic hook restores before chaining to the prior handler; the long-lived
`TerminalRestoreGuard` restores on explicit exit and `Drop`; and the custom
terminal's `Drop` resets/shows the cursor
([`tui/src/tui.rs:429-432`](../../.reference/codex/codex-rs/tui/src/tui.rs#L429-L432),
[`tui/src/tui.rs:553-558`](../../.reference/codex/codex-rs/tui/src/tui.rs#L553-L558),
[`tui/src/lib.rs:987-997`](../../.reference/codex/codex-rs/tui/src/lib.rs#L987-L997),
[`tui/src/lib.rs:1763-1792`](../../.reference/codex/codex-rs/tui/src/lib.rs#L1763-L1792),
[`tui/src/custom_terminal.rs:152-169`](../../.reference/codex/codex-rs/tui/src/custom_terminal.rs#L152-L169)).
External interactive programs are handled deliberately: event polling pauses,
alternate screen is left, terminal/stderr modes are restored, and all are
reclaimed and redrawn afterward
([`tui/src/tui.rs:695-780`](../../.reference/codex/codex-rs/tui/src/tui.rs#L695-L780)).

## Initial visible state and branching (observed)

On an ordinary authenticated launch, the first visible state is not a loading
log. It is a provisional session header plus the real composer and cursor;
while startup proceeds, edits are accepted but prompt submission, commands,
and image access are withheld. New sessions show no loading row, while resume
and fork show a dim `Resuming session…` or `Forking session…` row
([`tui/src/startup_draft.rs:1-1`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L1),
[`tui/src/startup_draft.rs:81-97`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L81-L97),
[`tui/src/startup_draft.rs:375-390`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L375-L390),
[`tui/src/startup_draft.rs:471-500`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L471-L500)).
Session-picker and first-login/onboarding paths intentionally suppress the
composer until the protected surface is ready; buffered typeahead is drained
before protected actions can be confirmed
([`tui/src/startup_orchestration.rs:152-176`](../../.reference/codex/codex-rs/tui/src/startup_orchestration.rs#L152-L176),
[`tui/src/tui.rs:724-734`](../../.reference/codex/codex-rs/tui/src/tui.rs#L724-L734)).

Non-interactive behavior is a dispatch branch, not a degraded TUI. `exec` and
`review` use `codex_exec::run_main`; the interactive terminal initializer itself
errors unless stdin and stdout are terminals
([`cli/src/main.rs:1135-1179`](../../.reference/codex/codex-rs/cli/src/main.rs#L1135-L1179),
[`tui/src/tui.rs:421-428`](../../.reference/codex/codex-rs/tui/src/tui.rs#L421-L428)).

## Revolvr comparison and gap analysis (observed)

| Concern | Codex | Revolvr today | Gap |
|---|---|---|---|
| Default launch | No subcommand enters TUI ([`cli/src/main.rs:1090-1142`](../../.reference/codex/codex-rs/cli/src/main.rs#L1090-L1142)). | Root `RunE` displays help; TUI is explicit `revolvr tui` ([`internal/cli/root.go:95-103`](../../internal/cli/root.go#L95-L103), [`internal/cli/root.go:1681-1708`](../../internal/cli/root.go#L1681-L1708)). | Launch feel diverges before rendering starts. |
| Work before first frame | Terminal and startup composer are established before bootstrap futures ([`tui/src/startup_orchestration.rs:152-180`](../../.reference/codex/codex-rs/tui/src/startup_orchestration.rs#L152-L180)). | `app.Status` completes before `RunStatus` is called ([`internal/cli/root.go:1701-1708`](../../internal/cli/root.go#L1701-L1708)). | Slow state loading leaves the old shell visible. |
| Runtime ownership | Purpose-built TTY/mode/probe/event/viewport/restoration layer ([`tui/src/tui.rs:227-247`](../../.reference/codex/codex-rs/tui/src/tui.rs#L227-L247), [`tui/src/tui.rs:421-513`](../../.reference/codex/codex-rs/tui/src/tui.rs#L421-L513)). | `tea.NewProgram(...).Run()` receives input/output options; no terminal lifecycle code appears at this boundary ([`internal/tui/model.go:487-512`](../../internal/tui/model.go#L487-L512)). | Bubble Tea defaults, rather than a declared Revolvr contract, determine mechanics. |
| Screen model | Ratatui buffers + custom diff terminal + inline viewport/history insertion ([`tui/src/custom_terminal.rs:126-150`](../../.reference/codex/codex-rs/tui/src/custom_terminal.rs#L126-L150), [`tui/src/tui.rs:948-1048`](../../.reference/codex/codex-rs/tui/src/tui.rs#L948-L1048)). | `View` assembles one Lip Gloss string; `Init` emits committed cells using `tea.Println` ([`internal/tui/model.go:515-535`](../../internal/tui/model.go#L515-L535), [`internal/tui/model.go:951-960`](../../internal/tui/model.go#L951-L960)). | No explicit viewport/history ownership or synchronized redraw contract. |
| Initial content | Provisional header/composer, with protected first-run/picker alternatives ([`tui/src/startup_draft.rs:270-282`](../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L270-L282)). | Model commits a session cell plus historical run cells and activates the composer ([`internal/tui/model.go:465-484`](../../internal/tui/model.go#L465-L484)). | Existing runs can dominate the opening viewport, contrary to the accepted visual baseline. |
| Failure safety | Layered init guard, panic hooks, restoration guard, cursor `Drop` ([`tui/src/lib.rs:987-997`](../../.reference/codex/codex-rs/tui/src/lib.rs#L987-L997), [`tui/src/lib.rs:1763-1792`](../../.reference/codex/codex-rs/tui/src/lib.rs#L1763-L1792)). | `main` prints returned errors; TUI runner returns Bubble Tea's error ([`cmd/revolvr/main.go:14-23`](../../cmd/revolvr/main.go#L14-L23), [`internal/tui/model.go:496-512`](../../internal/tui/model.go#L496-L512)). | No app-owned restoration policy is visible in Revolvr. |
| Libraries | Rust: Clap, Tokio, Ratatui, Crossterm ([`cli/src/main.rs:1-4`](../../.reference/codex/codex-rs/cli/src/main.rs#L1-L4), [`tui/Cargo.toml:73-90`](../../.reference/codex/codex-rs/tui/Cargo.toml#L73-L90)). | Go: Cobra, Bubble Tea, Bubbles, Lip Gloss ([`go.mod:5-12`](../../go.mod#L5-L12)). | Library parity is neither necessary nor sufficient; behavioral contracts are missing. |

## Existing Bubble Tea capabilities and limits (observed)

Revolvr uses Bubble Tea `v1.3.4` ([`go.mod:5-8`](../../go.mod#L5-L8)). Its
first-party source shows that `Program.Run` initializes the terminal, enables
configured terminal modes, starts the renderer, calls `Model.Init`, and queues
`Model.View` as the initial render before subscribing to input and entering the
event loop
([`tea.go:518-647`](https://github.com/charmbracelet/bubbletea/blob/v1.3.4/tea.go#L518-L647)).
This is sufficient to paint a startup model before an asynchronous `Init`
command completes—but Revolvr currently performs `app.Status` before it creates
that model.

Bubble Tea's default renderer is already an inline managed region. On each
flush it moves to the top of its previously rendered lines, skips unchanged
lines, repaints changed lines, and clears stale content below; alternate screen
is opt-in through `WithAltScreen`
([`standard_renderer.go:160-290`](https://github.com/charmbracelet/bubbletea/blob/v1.3.4/standard_renderer.go#L160-L290),
[`options.go:95-113`](https://github.com/charmbracelet/bubbletea/blob/v1.3.4/options.go#L95-L113)).
It does not perform Codex's cell-level Ratatui buffer diff or wrap each update
in synchronized-update mode. That is a fidelity difference to measure after
the startup composition is corrected, not yet evidence for a replacement
renderer.

Bubble Tea also owns more lifecycle behavior than Revolvr's call site makes
visible: raw input and cursor hiding, bracketed-paste cleanup, cursor and input
restoration, panic recovery, signal handling, resize messages, and
release/restore around interactive child commands
([`tty.go:25-74`](https://github.com/charmbracelet/bubbletea/blob/v1.3.4/tty.go#L25-L74),
[`exec.go:101-127`](https://github.com/charmbracelet/bubbletea/blob/v1.3.4/exec.go#L101-L127)).
The remaining lifecycle work is to declare Revolvr's policy and test that this
upstream behavior satisfies it; app-owned low-level terminal code should be
added only for a failed acceptance test.

Finally, `tea.Println` explicitly publishes unmanaged, persistent lines above
an inline program and does nothing in alternate-screen mode
([`standard_renderer.go:753-784`](https://github.com/charmbracelet/bubbletea/blob/v1.3.4/standard_renderer.go#L753-L784)).
That API matches Revolvr's current transcript behavior exactly and explains why
removing startup calls to it is central to matching Codex's opening screen.

## Staged tracer-bullet plan (recommendation, not observed behavior)

Each stage should be independently demonstrable and preserve existing
non-interactive subcommands.

1. **Launch tracer:** make the eventual bare-command route and explicit TUI
   route share one dispatcher, create the Bubble Tea program before status
   loading, and render a minimal startup model immediately. Retain a
   deterministic non-interactive/help escape. Acceptance: a delayed fake status
   provider cannot delay terminal ownership or the first frame; PTY capture
   proves both entry routes share the behavior.
2. **Exact first-frame tracer:** render only the agreed Codex-like 80x24 launch
   composition (header, whitespace, prompt, model/CWD status, cursor) from a
   minimal startup state. Do not call `tea.Println` or load/print historical runs
   into that frame. Acceptance: side-by-side golden terminal captures at 80x24
   and a one-character draft survives bootstrap completion.
3. **Startup ownership tracer:** enter the shell before status/bootstrap work,
   keep draft editing responsive, prohibit submission until ready, and replace
   loading placeholders with a compact ready, error, or first-run state. This
   can be folded into stages 1-2 if the model proves smaller that way; do not
   create a second startup framework. Acceptance: errors stay inside the shell
   and never strand the terminal.
4. **Terminal lifecycle tracer:** declare TTY checks, inline versus alternate
   viewport policy, resize behavior, cursor/mode restoration, panic recovery,
   and external-process handoff. Keep ordinary launch inline to match the
   observed Codex trace. Prefer Bubble Tea facilities where they meet the
   contract; add lower-level code only for evidenced gaps. Acceptance: PTY
   byte-level lifecycle tests plus manual Ctrl-C/panic/resize checks.
5. **Transcript tracer:** append one completed operation to scrollback while
   repainting one active viewport, with resize-safe wrapping. Acceptance:
   committed rows survive exit in normal scrollback and are never duplicated.
6. **Presentation expansion:** add discovery and one overlay, then migrate
   Revolvr-specific workflows individually. Acceptance: every slice includes
   fixed-geometry visual evidence and lifecycle regression tests.

## Unknowns and decisions to validate

- Decide whether Revolvr should match Codex's current default (`auto` uses
  alternate screen when requested by a surface) or remain inline-only initially;
  Codex supports both, so “Codex-like” does not decide this
  ([`tui/src/lib.rs:1795-1807`](../../.reference/codex/codex-rs/tui/src/lib.rs#L1795-L1807)).
- Determine which Codex surface is the visual target for “first launch”:
  authenticated composer, login/onboarding, or untrusted-directory prompt.
  Source shows these are intentionally different branches
  ([`tui/src/startup_orchestration.rs:152-176`](../../.reference/codex/codex-rs/tui/src/startup_orchestration.rs#L152-L176)).
- Validate whether bare `revolvr` may change from help to TUI without breaking
  scripts, and define explicit behavior for redirected stdin/stdout. Codex's
  interactive initializer rejects non-TTY streams
  ([`internal/cli/root.go:95-103`](../../internal/cli/root.go#L95-L103),
  [`tui/src/tui.rs:421-428`](../../.reference/codex/codex-rs/tui/src/tui.rs#L421-L428)).
- Establish whether Bubble Tea's renderer can satisfy inline viewport,
  synchronized update, and restoration acceptance tests as configured, or
  whether Revolvr needs a narrow terminal adapter. The source and PTY audit
  confirms restoration and inline repainting are present, but synchronized
  updates and Codex's cell-diff algorithm are not. Decide from visible tearing
  and scrollback tests after fixing startup, not from implementation parity
  alone ([`internal/tui/model.go:487-512`](../../internal/tui/model.go#L487-L512)).
- Decide which Revolvr domain facts replace Codex model/CWD/account status and
  whether plain text creates a task, starts a conversational turn, or remains
  disabled. Visual parity does not answer backend semantics.

## Verification ideas

- Record raw PTY output and screenshots for Codex and Revolvr at 80x24 from a
  clean home, authenticated home, initialized project, and redirected-I/O run.
- Add deterministic model snapshots for the first frame, one-character draft,
  delayed-bootstrap completion, resize to 100x30 and 60x20, and one overlay.
- Add PTY assertions for raw-mode/cursor restoration after normal quit, Ctrl-C,
  injected panic, startup error, and child interactive command; after each,
  verify canonical input and echo work.
- Inspect escape sequences to prove whether normal launch uses inline viewport,
  when `?1049h/?1049l` alternate-screen transitions occur, and whether bracketed
  paste and focus modes are paired on every exit.
- Use a fake delayed status provider to assert first-frame latency is independent
  of repository history loading and that draft text survives ownership transfer.
- Test scrollback invariants: active frames replace in place; committed results
  append exactly once; resizing does not replay, erase, or duplicate history.

## Boundaries

- Reimplement observed presentation in the existing Go/Bubble Tea codebase.
- Do not copy, vendor, port, or embed Codex source.
- Do not add a dependency merely to obtain Codex styling.
- Do not infer chat/backend semantics from this terminal research.
- The retired `docs/architecture/tui-overhaul/` plan is not a source of tasks;
  the plan above is research guidance, not published canonical task state.
