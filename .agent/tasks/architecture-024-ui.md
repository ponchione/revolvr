---
id: architecture-024-ui
status: completed
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-023-sequential-queue
---

# Refine the terminal operator workflow

## Sequence and status

- Sequence: `024` of `025`.
- Status: completed on 2026-08-27.
- Prerequisite: `architecture-023-sequential-queue`.
- ADR-025 supersedes the former desktop/Wails/Vue/REST/SSE direction and the
  former Architecture 024 desktop phase gate.

## Primary outcome

Refine the existing Bubble Tea TUI into a focused, Codex-like operator
workflow while keeping canonical state and business logic in the existing Go
application services.

## Required reading

- ADR-025.
- `README.md` sections "Mixed-Pass Task Workflow" and "TUI".
- `internal/tui/model.go`, `internal/app/timeline.go`, and the existing app
  actions supplied to the TUI.

## Existing foundations to reuse

- The Bubble Tea, Bubbles, and Lip Gloss dependencies already in `go.mod`.
- `internal/tui` navigation, viewport, progress, needs-input, and cancellation
  behavior.
- `internal/app` status, run timeline, task/run views, receipt validation,
  preflight, execution, queue, and operator-response services.
- Existing ledger events, receipts, diffs, evidence, and artifact identities.

## Implementation requirements

- Make a transcript/run-event view the main operator surface, using canonical
  run events and existing projections rather than inferred TUI state.
- Add a command composer and explicit operator-response flow, including the
  existing typed needs-input answer path.
- Use a compact status/footer that keeps active task, run state, controls, and
  safety-relevant outcomes visible without dominating the transcript.
- Add a command palette or slash-command equivalent for existing actions.
- Provide focused diff, evidence, and approval views backed by existing
  application services and artifact identities.
- Keep navigation and actions keyboard-accessible, preserve cancellation and
  refresh behavior, and keep narrow-terminal layouts usable.
- Keep all business, lifecycle, scheduling, verification, approval, and
  completion logic outside `internal/tui`.

## Scope boundaries and non-goals

- Do not add a desktop GUI, Wails, Vue, TypeScript, a `web/` tree, an embedded
  web server, REST, SSE, or browser security/configuration.
- Do not clone, vendor, port, or depend on Codex source; its interaction style
  is inspiration only.
- Do not add dependencies unless an existing installed terminal component
  cannot meet a demonstrated requirement.
- Do not change canonical lifecycle or queue authority merely to support a
  presentation choice.

## Acceptance criteria

- An operator can follow a run as a transcript/event stream and reach the
  composer, typed response, diff, evidence, and approval flows by keyboard.
- Displayed state remains traceable to an existing app projection, ledger
  event, receipt, or artifact; refresh reproduces canonical state.
- Status/footer and command discovery remain usable at narrow widths.
- TUI tests cover the new navigation and one representative response/approval
  flow without duplicating application-service tests.
- No business logic, new dependency, web surface, or desktop runtime is added.

## Deterministic verification

```bash
test -z "$(gofmt -l internal/tui)"
go test ./internal/tui ./internal/app
go test ./...
go run ./cmd/revolvr tui --help
git diff --check
```

## Expected completion report

Report the TUI workflow changes, reused application services and dependencies,
focused keyboard/accessibility coverage, verification results, and confirmation
that no desktop/web surface or TUI-owned business logic was added.
