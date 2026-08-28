---
id: tui-010-prove-shell-composition
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
---

# TUI-010 — Prove Transcript-Shell Composition

- Status: Completed 2026-08-27
- Accepted E0 source: `19d80f8977dabae8c3bd1f8a0cf430879147efa8`
- Accepted source:
  [TUI-010 draft](../../docs/architecture/tui-overhaul/tasks/tui-010-prove-shell-composition.md)
- Epic:
  [E1 — Prove the terminal shell](../../docs/architecture/tui-overhaul/epics/e1-terminal-shell.md)
- Depends on:
  [accepted E0 exit gate](../../docs/architecture/tui-overhaul/epics/e0-product-contract.md#exit-gate)

## Outcome

Demonstrate that the accepted history/live/composer composition works with the
installed Bubble Tea stack and Revolvr's program IO.

## Scope

- Build the smallest proof with `session-start` plus one canonical committed
  source cell, one replaceable live cell, and the accepted bottom composer.
- Copy the literal normal-width source rows and ownership from the
  [accepted snapshots](../../docs/architecture/tui-overhaul/README.md#accepted-experience-state-snapshots);
  introduce no alternate wording in the proof.
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

## Completion Evidence

- `internal/tui/model_test.go` contains the test-only proof. It emits the
  accepted `session-start` and completed cells once through installed
  `tea.Println`, retains semantic identities, and keeps only live/composer/
  footer rows in `View`.
- `gofmt -w internal/tui/model.go internal/tui/model_test.go` — PASS.
- `go test ./internal/tui -run 'TestTranscriptShellProof'` — PASS.
- `go test ./internal/tui` — PASS.
- `go test ./...` — PASS.
- Interactive PTY command — PASS:

  ```bash
  go test -c -o /tmp/revolvr-tui-proof.NoORaI/tui-proof.test ./internal/tui
  REVOLVR_TUI_INTERACTIVE_PROOF=1 /tmp/revolvr-tui-proof.NoORaI/tui-proof.test -test.run '^TestTranscriptShellProofInteractive$' -test.v
  ```

  After `q`, the proof disabled bracketed paste and mouse modes, restored the
  cursor, exited cleanly, and returned to Bash. `printf 'PROMPT_OK\n'` then ran
  successfully at that prompt. The temporary proof binary was removed.
