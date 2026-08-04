---
id: architecture-001-canonical-adrs
status: completed
workflow: mixed-pass-v1
phase: simplify
---

# Materialize the canonical architecture decisions as ADRs

## Sequence and status

- Sequence: `001` of `025`.
- Status: completed; this historical authority must not be scheduled again.
- Prerequisites: none.
- Phase gate: Phase 0 architecture authority must exist before repository or
  implementation decisions rely on it.

## Objective

Create the architecture-decision baseline required by Phase 0 of
`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`. Convert every decision in Section 4,
"Canonical Architecture Decisions," into one accepted, repository-owned ADR.
This task has one outcome: a complete, reviewable, one-to-one ADR set.

## Required reading

Before editing, read:

1. `AGENTS.md`.
2. `README.md`.
3. Section 4, "Canonical Architecture Decisions," of
   `REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`.
4. Phase 0 in Section 29 and the task-generation rules in Section 33 of the
   same specification.

Treat the canonical specification as source authority. Do not infer decisions
from the existing implementation when it conflicts with the specification.

## Existing foundations to inspect

- `docs/adr/` and its index when auditing the completed result.
- `.agent/DECISIONS.md` for older decisions that must not override the
  canonical Section 4 ADRs.
- The repository dependency and package layout only to detect contradictions;
  this task does not change implementation.

## Starting assumptions

- Section 4 contains exactly 24 accepted decisions, `ADR-001` through
  `ADR-024`.
- The canonical specification is the source authority even where the legacy
  implementation differs.
- This task records decisions; later tasks own every implementation change.

## Scope

- Create `docs/adr/README.md` as the ordered ADR index.
- Create exactly 24 ADR files under `docs/adr/`, one for each `ADR-001` through
  `ADR-024` in Section 4.
- Name each file `NNN-<short-kebab-title>.md`, preserving the three-digit ADR
  number from the specification.
- Give every ADR its canonical title, status `Accepted`, date `2026-08-03`, and
  an explicit source reference to Section 4 of the canonical specification.
- Record the decision, its relevant context, and its direct consequences in a
  compact conventional ADR structure. Preserve all normative requirements,
  prohibitions, defaults, and deferrals stated by the source decision.
- Link all 24 ADRs from `docs/adr/README.md` in numeric order.

## Boundaries

- Do not modify `REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md` while implementing
  this task.
- Do not implement any ADR in this task.
- Do not change Go, SQL, frontend, build, CI, configuration, dependencies, or
  runtime-state files.
- Do not remove Shunter, LanceDB, SQLite, provider, or orchestration code yet.
- Do not invent alternatives, reopen settled decisions, or add speculative
  architecture. If the specification deliberately defers a choice, preserve
  that deferral.
- Do not decide the separate Phase 0 question of in-place evolution versus a
  clean rewrite branch; ADR-001 explicitly leaves repository history open.
- Keep the ADRs independently readable, but avoid duplicating unrelated parts
  of the full specification.

## Acceptance criteria

- `docs/adr/` contains one and only one numbered ADR for every number from
  `001` through `024`.
- Each ADR maps to exactly one Section 4 decision and preserves its meaning
  without contradiction or scope expansion.
- Each ADR contains its number/title, accepted status, date, source, context,
  decision, and consequences.
- The index links every ADR once and in numeric order.
- The canonical specification is unchanged by the implementation commit.
- The diff contains documentation only: the ADR set, its index, and the
  harness-owned metadata transition for this task.

## Verification

Run:

```bash
test "$(find docs/adr -maxdepth 1 -type f -name '[0-9][0-9][0-9]-*.md' | wc -l)" -eq 24
for n in $(seq -f '%03g' 1 24); do
  test "$(find docs/adr -maxdepth 1 -type f -name "$n-*.md" | wc -l)" -eq 1 || exit 1
done
git diff --check
```

Then manually compare the ADR index and every ADR against Section 4 to confirm
that no decision, prohibition, or explicit deferral was omitted or changed.

## Completed provenance

- Implementation commit:
  `fefda56b596376313da58ddaf2b76f1f15c5784e` (`Record canonical architecture decisions`).
- The commit added the 24 ADRs and `docs/adr/README.md`; it did not implement
  any architecture feature.

## Expected completion report

Report the implementation commit, all ADR files and the index, the contiguous
`001`-`024` check, the Section 4 comparison, and confirmation that no product
code or canonical requirements changed.
