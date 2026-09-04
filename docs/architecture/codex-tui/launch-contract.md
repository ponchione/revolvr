# Ordinary Initialized Launch Contract

- Status: Accepted 2026-09-04 by CTUI-001
- Scope: ordinary authenticated Codex and initialized Revolvr launch only
- Standard geometry: `standard-80x24`
- Narrow geometry: `narrow-60x20`
- Research context: [Codex TUI baseline](../codex-tui-baseline.md)

This document locks the launch and initial-frame decisions consumed by
CTUI-010 and CTUI-020. Sections labeled **Observed** describe fresh executable
evidence or cited source. Sections labeled **Revolvr decision** are product
requirements. An observed Codex behavior is not a Revolvr requirement unless a
decision section adopts it.

Uninitialized workspaces and startup errors have no fixture here. CTUI-025
exclusively owns those fixtures and their retry, diagnostic, and exit behavior.

## Fresh evidence

### Method

**Observed.** On 2026-09-04, a clean local `main` at
`ac61907f7469f8a5836e9ee57a59066c854f2b4d` was one commit ahead of
`origin/main`. `codex --version` returned `codex-cli 0.153.2`, `codex login
status` returned `Logged in using ChatGPT`, and `revolvr config check`
confirmed the current checkout was initialized. The pinned source citation
checkout was `.reference/codex` at
`8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`.

Each executable was spawned afresh in a pseudo-terminal with
`TERM=xterm-256color`, `COLORTERM=truecolor`, the named row and column count,
and `/home/gernsback/source/revolvr` as its working directory. The harness
answered cursor-position, foreground/background-color, device-attribute, and
keyboard-enhancement probes like an ordinary terminal, recorded timestamped
raw output, waited four seconds, sent Ctrl-C, and required a normal exit.
Revolvr was built from that HEAD to `/tmp` before capture; no capture helper or
artifact was added to the repository.

| Application and route | Geometry | Started (UTC) | Raw bytes | Raw SHA-256 | Exit |
|---|---:|---:|---:|---|---:|
| bare `codex` | 80x24 | 2026-09-04 15:34:46 | 7,716 | `be8379b82ccec1bc48dd920a9e744b7aad58a4a83b885555f94faba34c0f21fe` | 0 |
| bare `codex` | 60x20 | 2026-09-04 15:34:50 | 7,809 | `f8f977e39f5eb610b2704841a52d90a3229e58125656e794f26a90fca9d7f252` | 0 |
| `revolvr tui` | 80x24 | 2026-09-04 15:34:54 | 2,593 | `63cc67826430496e913ed865a75aa8be7f180007fa97e1569d7437a99c7ddb61` | 0 |
| `revolvr tui` | 60x20 | 2026-09-04 15:34:59 | 2,536 | `b5c2925ce45ebd82dbff468b40fa4827dad8a41e38424cef8623f7970fdafcc2` | 0 |
| bare `revolvr` | 80x24 | 2026-09-04 15:39:11 | 2,075 | `56fa6586c29a851c6342273c29ba9513bb81a32487829e6fc677af36d2c7ec07` | 0 |
| bare `revolvr` | 60x20 | 2026-09-04 15:39:14 | 2,075 | `56fa6586c29a851c6342273c29ba9513bb81a32487829e6fc677af36d2c7ec07` | 0 |

The hashes identify the complete raw runs, including control sequences and
environment-specific Codex notices. The fixtures below transcribe only the
ordinary shell states being adopted; they do not turn volatile notices into
fixture content.

### Results

**Observed Codex.** In both geometries, bare Codex first painted the session
card with literal `model: loading` and `directory: loading`, the real composer,
and its shortcut footer. The directory then resolved, followed by the model
and reasoning effort. The 80x24 run painted its first synchronized loading
frame at about 274 ms; the 60x20 run did so at about 273 ms. Source creates the
terminal and provisional header before bootstrap and pumps terminal events
alongside startup work
([`startup_draft.rs:99-143`](../../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L99-L143),
[`startup_draft.rs:202-224`](../../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L202-L224)).

This machine's Codex configuration also produced a rotating tip, a usage
notice, and a failed `railway` MCP startup notice. Those are observed
environment-specific transcript content, not part of either ordinary shell
fixture and not adopted by Revolvr. The stable shell remained editable and
reached the resolved model/project footer in both runs.

**Observed Revolvr.** Bare Revolvr printed Cobra help and exited zero in both
PTYs. Explicit `revolvr tui` synchronously loaded status before Bubble Tea took
ownership, then emitted the session cell and latest historical run. Its only
captured visible frame arrived at about 126 ms and was dominated by the prior
run rather than a loading shell. This follows the current dispatch and model
construction path
([`root.go:95-132`](../../../internal/cli/root.go#L95-L132),
[`root.go:1681-1710`](../../../internal/cli/root.go#L1681-L1710),
[`model.go:465-516`](../../../internal/tui/model.go#L465-L516)).

**Observed redirected I/O.** Fresh isolated redirects produced these results:

| Current invocation | Redirect | Current result |
|---|---|---|
| bare `codex` | stdin | stderr `Error: stdin is not a terminal`; stdout empty; exit 1 |
| bare `codex` | stdout | stderr `Error: stdout is not a terminal`; redirected stdout empty; exit 1 |
| bare `revolvr` | stdin and stdout | root help on stdout; stderr empty; exit 0 |
| `revolvr tui` | stdin | terminal control bytes preceded a cancel-reader error; exit 1 |
| `revolvr tui` | stdout | TUI bytes went to the file and the process needed Ctrl-C; normal signal handling returned 0 |

Codex source checks stdin before stdout and rejects either non-terminal stream
([`tui.rs:421-428`](../../../.reference/codex/codex-rs/tui/src/tui.rs#L421-L428)).
Bubble Tea may open a controlling TTY for its default non-terminal input, but
Revolvr supplies a custom input and output and currently declares no gate. The
new contract below is therefore an explicit Revolvr policy, not an assumption
about library defaults.

## Literal observed Codex fixtures

**Observed.** Spaces inside each card are literal. Style and cursor placement
are specified after the text fixtures. Each block contains occupied rows 1
through 11; every later row in its named geometry is blank. Volatile tips,
usage notices, MCP progress, and MCP failures are intentionally outside these
ordinary shell fixtures.

### `standard-80x24` loading

```text
╭───────────────────────────────────────╮
│ >_ OpenAI Codex (v0.153.2)            │
│                                       │
│ model:     loading   /model to change │
│ directory: loading                    │
╰───────────────────────────────────────╯


› Ask Codex to do anything

  ? for shortcuts
```

### `standard-80x24` resolved ready

```text
╭─────────────────────────────────────────────────╮
│ >_ OpenAI Codex (v0.153.2)                      │
│                                                 │
│ model:     gpt-5.6-sol xhigh   /model to change │
│ directory: ~/source/revolvr                     │
╰─────────────────────────────────────────────────╯


› Ask Codex to do anything

  gpt-5.6-sol xhigh · ~/source/revolvr
```

### `narrow-60x20` loading

```text
╭───────────────────────────────────────╮
│ >_ OpenAI Codex (v0.153.2)            │
│                                       │
│ model:     loading   /model to change │
│ directory: loading                    │
╰───────────────────────────────────────╯


› Ask Codex to do anything

  ? for shortcuts
```

### `narrow-60x20` resolved ready

```text
╭─────────────────────────────────────────────────╮
│ >_ OpenAI Codex (v0.153.2)                      │
│                                                 │
│ model:     gpt-5.6-sol xhigh   /model to change │
│ directory: ~/source/revolvr                     │
╰─────────────────────────────────────────────────╯


› Ask Codex to do anything

  gpt-5.6-sol xhigh · ~/source/revolvr
```

The literal loading values come from the provisional constructor
([`startup_draft.rs:452-468`](../../../.reference/codex/codex-rs/tui/src/startup_draft.rs#L452-L468)).
The title, labels, resolved model/reasoning, change hint, path abbreviation,
padding, and border are constructed by the session-header cell
([`session.rs:282-300`](../../../.reference/codex/codex-rs/tui/src/history_cell/session.rs#L282-L300),
[`session.rs:312-375`](../../../.reference/codex/codex-rs/tui/src/history_cell/session.rs#L312-L375)).
Codex renders the `›` composer, dim placeholder, and cursor independently of
that header
([`chat_composer.rs:4880-4939`](../../../.reference/codex/codex-rs/tui/src/bottom_pane/chat_composer.rs#L4880-L4939)).

## Accepted Revolvr fixtures

**Revolvr decision.** These are the only accepted ordinary initialized launch
fixtures. The fixture values are Revolvr version `dev`, effective Codex model
`gpt-5.6-sol`, reasoning effort `xhigh`, and canonical project root
`/home/gernsback/source/revolvr`, abbreviated relative to the current home.
Tests may substitute values only when they preserve the field rules below.

The Revolvr card remains 41 columns at both accepted geometries. Omitting
Codex's unsupported `/model` affordance removes the content that caused the
observed ready card to grow. Rows 12-24 at 80x24 and rows 12-20 at 60x20 are
blank.

### `standard-80x24` loading

```text
╭───────────────────────────────────────╮
│ >_ Revolvr (dev)                      │
│                                       │
│ model:   loading                      │
│ project: loading                      │
╰───────────────────────────────────────╯


› Describe a task

  ? for shortcuts
```

### `standard-80x24` resolved ready

```text
╭───────────────────────────────────────╮
│ >_ Revolvr (dev)                      │
│                                       │
│ model:   gpt-5.6-sol xhigh            │
│ project: ~/source/revolvr             │
╰───────────────────────────────────────╯


› Describe a task

  gpt-5.6-sol xhigh · ~/source/revolvr
```

### `narrow-60x20` loading

```text
╭───────────────────────────────────────╮
│ >_ Revolvr (dev)                      │
│                                       │
│ model:   loading                      │
│ project: loading                      │
╰───────────────────────────────────────╯


› Describe a task

  ? for shortcuts
```

### `narrow-60x20` resolved ready

```text
╭───────────────────────────────────────╮
│ >_ Revolvr (dev)                      │
│                                       │
│ model:   gpt-5.6-sol xhigh            │
│ project: ~/source/revolvr             │
╰───────────────────────────────────────╯


› Describe a task

  gpt-5.6-sol xhigh · ~/source/revolvr
```

### Field, loading, and omission mapping

**Revolvr decision.** One successful bootstrap atomically replaces loading
with ready. Partial bootstrap values do not create additional accepted frames.

| Observed Codex element | Revolvr field and authority | Loading treatment | Ready omission treatment |
|---|---|---|---|
| `>_ OpenAI Codex` | literal `>_ Revolvr` product identity | always available | never omitted |
| `(v0.153.2)` | root command's normalized build version; empty becomes `dev` ([`root.go:84-105`](../../../internal/cli/root.go#L84-L105)) | always available | never omitted |
| `model:` value | validated effective `CodexModel` followed by `CodexReasoningEffort` | one italic `loading` value for the combined field | model is required; omit an absent reasoning effort and its preceding space |
| `/model to change` | no Revolvr equivalent; configuration remains outside this initial shell | omitted | omitted; never show an unavailable affordance or reserve padding for it |
| `directory:` value | label `project:` and canonical `StatusResult.ProjectRoot` ([`app.go:195-205`](../../../internal/app/app.go#L195-L205), [`app.go:238-245`](../../../internal/app/app.go#L238-L245)) | one italic `loading` value | required; inability to resolve it is a CTUI-025 startup error, not omission |
| home-relative directory | project path uses `~` only when it is beneath the current user's home; otherwise it is absolute | not applicable | center-elide only if needed to fit the 39-cell card interior |
| `›` and placeholder | literal `› Describe a task`; this truthfully names Revolvr's reviewed task-draft input rather than claiming chat semantics | visible and editable; submission and non-editor actions are gated | visible, focused, and retains the same draft and cursor |
| loading shortcut footer | literal `? for shortcuts` | shown until the complete ready projection is installed | replaced atomically by ready status |
| resolved status footer | effective model, optional reasoning effort, literal dim ` · `, and displayed project path | unavailable as a whole | model and project are required; only absent reasoning is omitted |
| Codex tips, usage/account notices, MCP progress, and MCP failures | no ordinary-launch Revolvr field | omitted | omitted; startup diagnostics belong to CTUI-025 and later results belong to later transcript work |
| startup history | no ordinary-launch field | omitted | omitted; no prior run or task narrative is emitted during launch |

The effective model and reasoning authority already belongs to Revolvr's
validated run configuration; CTUI-020 may project it into the startup result
but must not create a second configuration source. An invalid or unreadable
configuration does not produce a partial ready frame.

### Layout, style, focus, and draft

**Revolvr decision.** CTUI-020 must apply these rules so it need not make a new
visual decision:

- The shell is inline, begins at column 1, and uses no alternate screen.
- The card occupies rows 1-6. Rows 7-8 are blank, the composer is row 9, row
  10 is blank, and the footer is row 11.
- The card border, `>_`, version, labels, placeholder, footer hint, and ready
  separator are dim. `Revolvr` is bold. Resolved values use default foreground.
  `loading` is dim and italic. The `›` is bold.
- The card has a 39-cell interior and one leading and trailing padding cell.
  The labels align their values at column 12, leaving a 28-cell value span.
  Longer model text is end-elided; longer project paths are center-elided.
  Neither accepted geometry truncates the fixture values above.
- The terminal cursor is visible at row 9, column 3, over the first placeholder
  cell when the draft is empty. Typed text replaces the placeholder without
  moving the `›`.
- Loading accepts ordinary draft edits and local cursor movement. It does not
  submit, invoke slash commands, open modal input, or start work. Ctrl-C may
  exit. The complete text, cursor, and focus transfer exactly once to ready.
- Ready retains existing reviewed task-draft semantics; this task changes no
  application action or domain authority.

## Accepted launch and I/O matrix

**Revolvr decision.** `stdin TTY` and `stdout TTY` refer to the effective
streams supplied to the command. Revolvr does not reopen `/dev/tty`, consume
redirected input, degrade into help, or emit terminal bytes before deciding
this gate. The stdin check is first, making the both-redirected result
deterministic.

| Invocation | stdin | stdout | Route | stdout before/during normal launch | stderr | Normal exit |
|---|---|---|---|---|---|---:|
| bare `revolvr` | TTY | TTY | shared ordinary TUI dispatcher | managed inline TUI only | empty | 0 after normal user quit |
| `revolvr tui` | TTY | TTY | same dispatcher and same model | byte-equivalent to bare route for the same state | empty | 0 after normal user quit |
| bare `revolvr` | redirected | TTY | reject before bootstrap or terminal ownership | empty | `stdin is not a terminal\n` | 1 |
| `revolvr tui` | redirected | TTY | same rejection | empty | `stdin is not a terminal\n` | 1 |
| bare `revolvr` | TTY | redirected | reject before bootstrap or terminal ownership | empty | `stdout is not a terminal\n` | 1 |
| `revolvr tui` | TTY | redirected | same rejection | empty | `stdout is not a terminal\n` | 1 |
| bare `revolvr` | redirected | redirected | stdin-first rejection | empty | `stdin is not a terminal\n` | 1 |
| `revolvr tui` | redirected | redirected | same stdin-first rejection | empty | `stdin is not a terminal\n` | 1 |

An error returned after terminal ownership is restored still exits 1 through
the existing main boundary. Its in-shell startup presentation is CTUI-025 work,
not another route in this ordinary initialized matrix.

### Help, version, parsing, and existing subcommands

**Revolvr decision.** Cobra parsing happens before the ordinary TUI gate:

- `revolvr -h`, `revolvr --help`, and `revolvr help` print root help to stdout
  and exit 0 for every stdin/stdout topology. They never bootstrap or launch
  the TUI.
- `revolvr tui -h`, `revolvr tui --help`, and `revolvr help tui` print TUI help
  to stdout and exit 0 for every stream topology. They never enter the TTY gate.
- `revolvr --version` retains Cobra's existing version route: `revolvr
  <version>\n` on stdout, empty stderr, and exit 0. There is no `version`
  subcommand; `revolvr version` remains an unknown-command error on stderr and
  exits 1.
- Unknown commands, positional arguments to either TUI entry, and malformed
  flags retain Cobra's current parse error on stderr and exit 1. They do not
  launch the TUI or print usage because `SilenceUsage` remains true.
- These existing non-TUI top-level commands and every existing nested command
  remain exact, explicit dispatch routes: `archive`, `artifact`, `checkpoint`,
  `config`, `doctor`, `init`, `ledger`, `metrics`, `notification`, `queue`,
  `receipt`, `run`, `show`, `status`, and `task`. Their arguments, flags,
  stdout/stderr, side effects, and exit status remain those at baseline HEAD;
  neither TTY topology nor the new root default reroutes them through the TUI.

The complete accepted root help text is:

```text
Run bounded Codex harness passes

Usage:
  revolvr [flags]
  revolvr [command]

Available Commands:
  archive      Archive, inspect, verify, and reopen terminal tasks
  artifact     Plan, apply, and inspect artifact retention
  checkpoint   Manage pre-authored operator checkpoints
  config       Inspect run configuration
  doctor       Check readiness for dogfooding
  help         Help about any command
  init         Initialize revolvr state
  ledger       Export and validate immutable ledger history
  metrics      Project autonomous-loop metrics from ledger evidence
  notification Inspect durable external notification deliveries
  queue        Manage manually started bounded sequential queues
  receipt      Inspect and validate receipts
  run          Run one harness pass
  show         Show one run
  status       Show harness status
  task         Manage tasks
  tui          Open the Revolvr TUI

Flags:
  -h, --help      help for revolvr
  -v, --version   version for revolvr

Use "revolvr [command] --help" for more information about a command.
```

The complete accepted `revolvr tui --help` text is:

```text
Open the Revolvr TUI.

Bare revolvr and revolvr tui are equivalent when stdin and stdout are
terminals. Use an existing subcommand for non-interactive work.

Usage:
  revolvr tui [flags]

Flags:
  -h, --help   help for tui
```

Existing non-TUI command help remains generated from the unchanged command
definitions registered at
[`root.go:115-132`](../../../internal/cli/root.go#L115-L132). CTUI-010 replaces
only the retired transcript-first TUI description with the literal help above.

## Ownership and task boundary

**Revolvr decision.** The shared dispatcher owns the launch in this order:

1. Let Cobra resolve help, version, parse errors, and explicit non-TUI
   subcommands.
2. For bare or explicit TUI launch, check effective stdin, then stdout. A
   refusal performs no workspace/config/status read and writes no terminal
   control sequence.
3. Create one Bubble Tea program in inline mode and let it own input, output,
   cursor, and terminal modes before starting status/config bootstrap.
4. Start bootstrap asynchronously only after the first render opportunity.
   Keep stderr empty on a successful ordinary launch and emit no startup
   `tea.Println` history.
5. Deliver one complete initialized bootstrap result to the same program. The
   model, terminal, draft, cursor, and focus are not replaced by a second
   program.

CTUI-010 owns steps 1-4, shared dispatch, the TTY refusal, asynchronous result
delivery, and removal of startup history emission. Its **minimal pending
frame** means only a successful nonempty Bubble Tea render before delayed
bootstrap completes. It may use one unstyled `Loading…` line as a disposable
tracer marker. That marker has no card, composer, footer, draft, style, or
product-state semantics; it is not an accepted visual fixture or a golden, and
CTUI-020 must replace it rather than preserve it.

CTUI-020 owns the two accepted Revolvr fixtures, styling, loading-to-ready
replacement, draft editing and gating, and lossless transfer. It may not add a
third partial-loading frame, startup transcript, alternate-screen transition,
or new field. CTUI-025 alone owns uninitialized and startup-error fixtures.

This boundary lets CTUI-010 prove early terminal ownership without choosing
lasting presentation or draft behavior, while CTUI-020 can implement the
ordinary initialized visuals without reopening launch or field decisions.
