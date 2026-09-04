# CTUI-065 — Lock the First Focused Surface

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-050 and CTUI-060

## Outcome

Select and lock one focused surface and its viewport policy from fresh evidence.

## Scope

- Select exactly one smallest useful Revolvr surface using fresh side-by-side
  evidence rather than invention.
- Record literal normal and narrow fixtures plus entry, completion, dismissal,
  focus, cursor, draft, active-state, and scrollback behavior.
- Decide inline versus alternate-screen policy from the selected surface's
  evidence and requirements.

## Acceptance

- One surface is selected with a documented evidence-based rationale.
- Fixtures and transition matrices settle both completion and dismissal paths.
- Viewport policy is explicit and does not assume alternate screen.
- CTUI-070 can implement without selecting a surface, fixture, or policy.

## Verification

- Review fresh side-by-side captures and source citations.
- Check every entry and return-state field against the fixture and policy.

## Not Included

- Product code, dependency changes, or canonical publication.
- Surface implementation, a second surface, bulk migration, or launch and
  command-discovery redesign.
