# CTUI-060 — Implement Command Discovery

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-055

## Outcome

Implement the locked command-discovery contract in the ready composer.

## Scope

- Reproduce CTUI-055 fixtures, entry paths, labels, selection, navigation,
  dismissal, and Revolvr mapping exactly.
- Preserve draft, cursor, eligibility rules, and existing action guards.
- Cover the locked normal and narrow geometries.

## Acceptance

- Snapshots match all locked discovery states at both geometries.
- Entry, navigation, selection, and dismissal follow the locked matrix.
- Dismissal preserves draft and cursor; selection cannot bypass existing guards.
- Focused interaction evidence fits one fresh pass.

## Verification

- Run deterministic snapshots and interaction tests for the locked contract.
- Capture entry through dismissal in a fixed-size PTY.
- Run `go test ./...`.

## Not Included

- Fixture or policy selection during implementation, command semantic changes,
  focused surfaces, broad restyle, new dependencies, or retired-plan
  requirements.
