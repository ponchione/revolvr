---
id: ptc-001-amend-task-specifications
status: completed
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-016-tool-broker-implementer
---

# Amend the active and deferred task specifications

## Sequence and status

- Sequence: supplemental integration planning before `016a` and `017`.
- Status: completed.
- Prerequisite: the completed architecture-016 direct-tool broker and
  implementer are preserved unchanged.
- Phase gate: this is a planning-only task; it adds no product implementation,
  runtime dependency, migration, Python service, harness asset, or Graphiti
  work.

## Primary outcome

Integrate the approved contract-level portions of
`REVOLVR_PROGRAMMATIC_WORKSPACE_AND_CONTINUAL_HARNESS_SPEC.md` into the active
architecture sequence, and record the implementation portions as explicitly
deferred, phase-gated work subordinate to the accepted ADRs and canonical
architecture.

## Acceptance evidence

- Architecture tasks 001-016 remain complete and architecture 016a is the next
  and only pending task.
- Architecture 017 depends on 016a; architectures 018, 021, and 022 contain the
  approved future-compatibility contracts without implementing post-core work.
- Architectures 019, 020, 023, 024, and 025 remain substantively unchanged.
- PTC-101 through PTC-108B are recorded as blocked/deferred and cannot be
  selected before architecture 001-025 and initial core-loop dogfooding
  evidence satisfy their phase gate.
- The staged base-workspace-before-skills evaluation sequence and the explicit
  absence of any Graphiti inference are durable authority.
- Durable selectors, state, and handoff agree, `git diff --check` passes, and
  no product surface changes as part of this planning task.
