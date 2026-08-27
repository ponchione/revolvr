# Technical Debt

## Architecture 024 TUI audit — 2026-08-27

All required formatting, test, CLI-help, task-list, status, and diff checks
passed. The following behavioral and completion-state gaps remain.

### High

1. **Bind operator confirmation to an immutable question snapshot.** A current
   autonomous-view reload can replace the question and option list without
   clearing an active answer, while confirmation indexes the replacement view
   ([`internal/tui/model.go`](internal/tui/model.go#L408),
   [`internal/tui/model.go`](internal/tui/model.go#L1089)). This can panic when
   options shrink or submit a newly loaded choice the operator did not confirm.
   Close this by retaining the selected task/question/revision/content hash and
   option ID through confirmation, rejecting any changed projection, and adding
   a reload-during-confirmation regression test.

2. **Preserve cancellation settlement while the composer is open.** `/` is
   handled before active-run key protection, and composer `ctrl+c` quits without
   requesting cancellation or waiting for settlement
   ([`internal/tui/model.go`](internal/tui/model.go#L499),
   [`internal/tui/model.go`](internal/tui/model.go#L1774)). This contradicts the
   displayed `ctrl+c Cancel/Quit` control and durable completion claims. Close
   this by routing active-run composer quit through the existing cancel-and-wait
   path and testing all run modes.

3. **Reconcile Architecture 024 completion through the mixed-pass workflow.**
   The task is `completed` at `phase: implement`
   ([`.agent/tasks/architecture-024-ui.md`](.agent/tasks/architecture-024-ui.md#L3)),
   although the documented workflow completes only after audit, document, and
   simplify ([`README.md`](README.md#L265)). `task list` therefore reports its
   next state as `audit` while `status` selects Architecture 025. Close this
   through harness-owned phase transitions before treating Architecture 024 as
   a satisfied dependency.

### Medium

1. **Accept real space-key input in the command composer.** Bubble Tea emits a
   standalone space as `tea.KeySpace`, but the composer accepts only
   `tea.KeyRunes` ([`internal/tui/model.go`](internal/tui/model.go#L1789)). The
   documented `/answer <option-id>` command is therefore not normally typeable;
   its test sends the whole string as one synthetic rune event
   ([`internal/tui/architecture_024_test.go`](internal/tui/architecture_024_test.go#L106)).
   Close this by handling `KeySpace` and testing input as separate terminal key
   events.

2. **Use the run's canonical task identity in transcript status.** The compact
   line associates the latest run with the next runnable task instead of
   `run.TaskID` ([`internal/tui/model.go`](internal/tui/model.go#L2420)). Close
   this by rendering the run projection's task ID and testing deliberately
   different next-task and run-task identities.

3. **Reload focused run history on refresh.** Status refresh replaces the
   status snapshot, but focused views opened from Run Detail continue using
   cached `runDetails` ([`internal/tui/model.go`](internal/tui/model.go#L283),
   [`internal/tui/model.go`](internal/tui/model.go#L2404)). Close this by
   reopening the focused run through the existing app action and testing a
   changed canonical event set.

4. **Render an actual canonical diff or label the view as a summary.** The run
   branch shows only changed filenames, commit metadata, and event IDs; the
   autonomous branch only searches existing references for a kind containing
   `diff` ([`internal/tui/model.go`](internal/tui/model.go#L2432)). Current
   autonomous projections do not supply a diff reference. Close this by using
   an existing exact diff artifact projection, or by renaming and documenting
   the view as changed-file metadata rather than a diff.

### Low

1. Consolidate the duplicated autonomous/focused viewport scrolling switches
   at [`internal/tui/model.go`](internal/tui/model.go#L691).
2. Reuse one artifact label/path formatter instead of maintaining separate
   tables at [`internal/tui/model.go`](internal/tui/model.go#L2547) and
   [`internal/tui/model.go`](internal/tui/model.go#L3310).
3. Rename `focusAutonomous` to describe that it selects the focused projection
   source, not keyboard focus ([`internal/tui/model.go`](internal/tui/model.go#L87)).
