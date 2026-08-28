# TUI-022 — Reconcile Live State with Committed History

- Status: Completed 2026-08-28
- Epic: [E2 — Build semantic transcript cells](../epics/e2-semantic-transcript.md)
- Depends on: [TUI-021](tui-021-project-historical-runs.md)

## Outcome

Consolidate one settled operation's replaceable live presentation into its
canonical committed transcript result exactly once.

## Scope

- Give the active presentation a stable operation identity.
- Keep existing progress caps, detail compaction, and redaction behavior while
  progress is replaceable.
- On settlement or refresh, reconcile live state against the canonical app
  result using domain identity.
- Preserve the accepted `session-start` source and emitted identity across
  refresh and live settlement.
- Append the reconciled final cell only when its stable identity is not already
  in the process-local emitted set.
- Ignore late/stale messages that target an older operation identity.
- Cover completion, failure, cancellation, blocked, safety-stop, and
  needs-input terminal results.

## Acceptance

- Intermediate progress never becomes an exhaustive committed log.
- Settlement creates one coherent final story with no duplicate live/final
  transition.
- Refresh during or after settlement converges on the canonical projection.
- A stale asynchronous message cannot rewrite or clear a newer operation.
- No rendered-string or timestamp heuristic participates in reconciliation.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestLiveTranscript(Reconciles|RejectsStale)'
go test ./internal/tui
```

## Not Included

- No richer live-cell content, composer change, queue, or lifecycle authority.

## Completion Evidence

- Stable run and operation identities reconcile matching terminal results with
  the process-local emitted set; stale token or domain identities are ignored.
- The installed append/ack boundary retains live state until the final cell is
  emitted, then clears it and releases any delayed quit.
- Refresh during or after settlement converges without replaying the final cell
  or the process-local `session-start`.
- Focused live-reconciliation tests, the TUI package, and the full Go suite
  pass.
