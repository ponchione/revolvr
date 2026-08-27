# Codex TUI Behavioral Reference

- Codex commit: `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`
- Study date: 2026-08-27
- Scope: observable interaction, transcript, composer, overlay, terminal, and
  presentation contracts that can inform the Revolvr TUI overhaul
- Excluded: Codex provider, model, token, attachment, plugin, multi-agent, and
  thread machinery except where a small part establishes an in-scope contract

## Authority Boundary

**Observed Codex behavior:** the pinned checkout is evidence for behavior, not
product authority.

**Candidate Revolvr adaptation:** D1 permits a native Go/Bubble Tea
reimplementation of accepted behavior. No Rust source or snapshot body may be
copied or ported. Revolvr branding, domain semantics, application-service
authority, and installed terminal dependencies remain authoritative. See the
[accepted D1 decision](../README.md#d1--codex-fidelity-and-adr-025) and
[ADR-025](../../../adr/025-terminal-first-simplified-harness.md).

**Open product decision:** D2 through D6 remain open. This study records
evidence and candidate seams without selecting composer meaning, queued-input
semantics, transcript ownership, overlay order, or the session-header
lifecycle.

## Citation Format

Codex citations use this form:

`Codex@8228e9b867251f544a5e0c6c80bb5ebc9d5446a1 path:Lx-Ly (symbol/test/snapshot)`

The path is relative to `.reference/codex`. A claim that crosses an ownership
boundary cites both sides. Revolvr citations use `Revolvr path:Lx-Ly
(symbol/test)`. Snapshot names are cited only as proof identities; their bodies
are not reproduced.

## Reference Index

- [Interaction model](interaction-model.md): composition, ownership, cells,
  composer, overlays, questions, approvals, cancellation, and replay.
- [Terminal mechanics](terminal-mechanics.md): inline rendering, terminal
  history, reflow, lifecycle, styling, uncertainty, and proof.
- [Revolvr mapping](revolvr-mapping.md): current seams, classifications, tasks,
  decisions, and prioritized evidence gaps.

## Task Lookup

Every proof or implementation task from TUI-010 through TUI-072 is covered
below. “No useful Codex analog” means Revolvr’s domain or delivery process is
the only useful authority.

| Task | Relevant reference |
|---|---|
| [TUI-010](../tasks/tui-010-prove-shell-composition.md) | [Shell composition and focus](interaction-model.md#shell-composition-and-focus); [rendering ownership](terminal-mechanics.md#rendering-ownership) |
| [TUI-011](../tasks/tui-011-prove-resize-reflow.md) | [Resize, reflow, and width](terminal-mechanics.md#resize-reflow-and-width) |
| [TUI-012](../tasks/tui-012-prove-active-settlement.md) | [Live-to-committed settlement](interaction-model.md#live-to-committed-settlement) |
| [TUI-013](../tasks/tui-013-install-terminal-shell.md) | [Bubble Tea boundary](terminal-mechanics.md#bubble-tea-boundary) and [shell seam mapping](revolvr-mapping.md#contract-mapping) |
| [TUI-020](../tasks/tui-020-define-transcript-cells.md) | [Semantic cell categories](interaction-model.md#semantic-cell-categories) |
| [TUI-021](../tasks/tui-021-project-historical-runs.md) | Codex replay is relevant to lifecycle only; Revolvr’s [timeline projection mapping](revolvr-mapping.md#contract-mapping) is the content authority |
| [TUI-022](../tasks/tui-022-reconcile-live-history.md) | [Live-to-committed settlement](interaction-model.md#live-to-committed-settlement) and [replay](interaction-model.md#session-header-replay-and-refresh) |
| [TUI-030](../tasks/tui-030-make-composer-primary.md) | [Composer ownership and submission](interaction-model.md#composer-ownership-and-submission) |
| [TUI-031](../tasks/tui-031-implement-plain-text-input.md) | [Composer ownership and submission](interaction-model.md#composer-ownership-and-submission); D2/D5 and Revolvr domain prerequisites remain authoritative |
| [TUI-032](../tasks/tui-032-add-contextual-command-discovery.md) | [Commands and history](interaction-model.md#commands-and-history) |
| [TUI-040](../tasks/tui-040-render-live-operation.md) | [Live-to-committed settlement](interaction-model.md#live-to-committed-settlement) |
| [TUI-041](../tasks/tui-041-render-queued-input.md) | [Queued input and interruption](interaction-model.md#queued-input-and-interruption); Revolvr has no accepted queue contract yet |
| [TUI-050](../tasks/tui-050-add-overlay-shell.md) | [Overlays and focus transfer](interaction-model.md#overlays-and-focus-transfer) |
| [TUI-051](../tasks/tui-051-move-tasks-overlay.md) | [Overlay shell mechanics](interaction-model.md#overlays-and-focus-transfer); no useful Codex analog for task content |
| [TUI-052](../tasks/tui-052-move-runs-overlay.md) | [Overlay shell mechanics](interaction-model.md#overlays-and-focus-transfer); no useful Codex analog for run content |
| [TUI-053](../tasks/tui-053-move-preflight-overlay.md) | [Overlay shell mechanics](interaction-model.md#overlays-and-focus-transfer); no useful Codex analog for preflight content |
| [TUI-054](../tasks/tui-054-move-workflow-overlay.md) | [Overlay shell mechanics](interaction-model.md#overlays-and-focus-transfer); no useful Codex analog for workflow content |
| [TUI-055](../tasks/tui-055-move-change-summary-overlay.md) | [Overlay shell mechanics](interaction-model.md#overlays-and-focus-transfer); no useful Codex analog for Revolvr change-summary content |
| [TUI-056](../tasks/tui-056-move-evidence-overlay.md) | [Overlay shell mechanics](interaction-model.md#overlays-and-focus-transfer); no useful Codex analog for Revolvr evidence content |
| [TUI-057](../tasks/tui-057-move-approval-overlay.md) | [Approvals](interaction-model.md#approvals) plus Revolvr’s typed approval mapping |
| [TUI-058](../tasks/tui-058-move-needs-input-overlay.md) | [Typed questions](interaction-model.md#typed-questions) plus Revolvr’s typed-input mapping |
| [TUI-060](../tasks/tui-060-lock-geometry-snapshots.md) | [Resize, reflow, and width](terminal-mechanics.md#resize-reflow-and-width) and [defining proof](terminal-mechanics.md#defining-tests-and-snapshots) |
| [TUI-061](../tasks/tui-061-verify-terminal-scrollback.md) | [History insertion and native scrollback](terminal-mechanics.md#history-insertion-and-native-scrollback) |
| [TUI-062](../tasks/tui-062-verify-terminal-lifecycle.md) | [Terminal lifecycle and restoration](terminal-mechanics.md#terminal-lifecycle-and-restoration) |
| [TUI-063](../tasks/tui-063-verify-text-accessibility.md) | [Styling and text accessibility](terminal-mechanics.md#styling-and-text-accessibility) |
| [TUI-070](../tasks/tui-070-remove-dashboard-presentation.md) | [Current shell mapping](revolvr-mapping.md#contract-mapping); no useful Codex analog for deletion safety |
| [TUI-071](../tasks/tui-071-update-operator-docs.md) | No useful Codex analog; Revolvr operator behavior is authoritative |
| [TUI-072](../tasks/tui-072-close-overhaul-acceptance.md) | [Evidence gaps](revolvr-mapping.md#prioritized-evidence-gaps); no useful Codex analog for Revolvr acceptance closure |

## Detecting Stale Evidence

Before using a citation, run:

```bash
test "$(git -C .reference/codex rev-parse HEAD)" = \
  8228e9b867251f544a5e0c6c80bb5ebc9d5446a1
```

A missing checkout, a different commit, a missing cited path, or a cited line
range whose named symbol/test no longer occupies that range makes the evidence
stale. Stop and create a new bounded study against an explicitly accepted pin;
do not silently move citations or update the checkout during implementation.
