# TUI-010 — Prove Transcript-Shell Composition

- Status: Draft; first implementation candidate after E0
- Epic: [E1 — Prove the terminal shell](../epics/e1-terminal-shell.md)
- Depends on: [E0 exit gate](../epics/e0-product-contract.md#exit-gate)

## Outcome

Demonstrate that the accepted history/live/composer composition works with the
installed Bubble Tea stack and Revolvr's program IO.

## Scope

- Build the smallest proof with `session-start` plus one canonical committed
  source cell, one replaceable live cell, and the accepted bottom composer.
- Emit committed cell renderings once through the installed `tea.Println`
  boundary while keeping the live cell and composer in the managed frame.
- Exercise both Bubble Tea test output buffers and one interactive terminal.
- Keep the proof in current TUI files/tests unless one package-local proof file
  is materially clearer.
- Retain enough source state to redraw according to D3; rendered strings are
  not lifecycle authority.

## Acceptance

- Each committed identity is appended above the program exactly once; redraw
  does not duplicate either committed cell.
- `session-start` precedes canonical-history replay in captured output.
- Replacing live state does not append intermediate rows to history.
- The composer remains at the bottom, receives keys, and renders at accepted
  normal width.
- Captured test output retains deterministic committed lines without claiming
  native terminal navigation.
- Normal quit restores an ordinary usable prompt.
- The proof fails under the old dashboard composition and passes under the new
  composition.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestTranscriptShellProof'
go test ./internal/tui
```

Record the interactive terminal command and observed restoration result in the
task completion evidence.

## Not Included

- No semantic run projection, resize proof, cancellation proof, overlay,
  plain-text dispatch, terminal-history clearing, or terminal backend.
