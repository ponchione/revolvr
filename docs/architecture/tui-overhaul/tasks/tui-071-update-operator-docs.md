# TUI-071 — Update Operator Documentation

- Status: Draft; not canonical or runnable
- Epic: [E7 — Remove the old dashboard shell](../epics/e7-remove-dashboard.md)
- Depends on: [TUI-070](tui-070-remove-dashboard-presentation.md)

## Outcome

Make user-facing documentation and CLI Help describe the shipped transcript,
composer, command, overlay, scrollback, cancellation, and loop behavior.

## Scope

- Update README and TUI help text to the accepted names and interaction paths.
- Document plain-text behavior, including unavailable states, exactly as shipped.
- Document command discovery, overlay open/close, history navigation/copy,
  cancellation, quit settlement, needs-input, and known terminal limitations.
- Remove instructions for the deleted Dashboard/page/inactive-composer model.
- Keep architecture planning status separate from operator instructions.

## Acceptance

- A new operator can launch, discover actions, inspect focused views, answer a
  question, cancel work, and exit using only current documentation.
- Every documented command/key exists and every shipped command is discoverable.
- Limitations match E6 evidence without overclaiming terminal support.
- `--help` and README use the same product terms.
- No draft decision language appears as shipped behavior.

## Verification

```bash
go test ./internal/tui
go test ./internal/cli
go run ./cmd/revolvr tui --help
go run ./cmd/revolvr --help
git diff --check -- README.md internal/tui internal/cli docs
```

## Not Included

- No UI behavior, command, key binding, final acceptance run, or durable-state
  closeout.
