---
id: architecture-002-repository-build-baseline
status: completed
workflow: mixed-pass-v1
phase: simplify
depends_on: architecture-001-canonical-adrs
---

# Establish the canonical repository and build baseline

## Sequence and status

- Sequence: `002` of `025`.
- Status: completed; this historical authority must not be scheduled again.
- Prerequisite: `architecture-001-canonical-adrs`.
- Phase gate: Phase 0 decisions are accepted before the existing repository is
  named as the in-place implementation baseline.

## Objective

Complete architecture sequence item 002 from
`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`. Record the existing Go repository as
the in-place foundation for the canonical architecture and make its build,
test, and CI entrypoints explicit. This task documents and verifies the
baseline; it does not begin the PostgreSQL foundation or reorganize working
code.

## Required reading

Before editing, read:

1. `AGENTS.md`, `README.md`, and `go.mod`.
2. `docs/adr/001-product-name.md`, `docs/adr/005-go-pgx-sqlc.md`, and
   `docs/adr/020-cli-first-desktop-ui-second.md`.
3. Section 8, "Suggested Repository Structure," Phase 0 in Section 29, and
   Section 33, "Task Generation Rules," of
   `REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`.
4. `.github/workflows/ci.yml` and the existing top-level repository layout.

Treat the canonical specification and accepted ADRs as architecture authority.
Treat the working repository as the implementation baseline to preserve unless
they conflict.

## Existing foundations to inspect

- `cmd/revolvr/main.go`, `go.mod`, and the current `internal/` package layout.
- `.github/workflows/ci.yml` and existing build/test scripts.
- `docs/architecture/repository-build-baseline.md` when auditing the completed
  result.

## Starting assumptions

- Revolvr evolves in place and preserves useful Git history and working CLI
  behavior.
- Suggested directories are created just in time by the task that owns their
  contents; empty scaffolding is not a deliverable.
- Go `1.26.5` is the current repository baseline after the recorded follow-up
  update.

## Scope

- Create `docs/architecture/repository-build-baseline.md`.
- Record the Phase 0 repository-history decision: evolve Revolvr in place,
  preserving the current Git history and working CLI while later bounded tasks
  replace architecture that conflicts with the accepted ADRs.
- Document the current module name, Go 1.26.5 source floor, CLI entry point, and
  these canonical commands:
  - build: `go build ./...`
  - full test: `go test ./...`
  - local CLI inspection: `go run ./cmd/revolvr --help`
- Identify `.github/workflows/ci.yml` as the current CI baseline and summarize
  its build/test role without duplicating the workflow.
- Describe the repository-structure rule: preserve working directories and
  add the suggested Section 8 directories only when the bounded task that owns
  their contents is implemented. Do not create empty scaffolding.
- Add the new baseline document to the `README.md` Documentation list.

## Boundaries

- Do not modify `REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md` while implementing
  this task.
- Do not modify Go source, `go.mod`, `go.sum`, CI workflows, build scripts,
  dependencies, configuration, or runtime-state files.
- Do not add a Makefile that only wraps the canonical Go commands.
- Do not move packages or create empty `cmd`, `internal`, `db`, `prompts`,
  `schemas`, `sandbox`, `web`, `compose`, or `evals` scaffolding for future
  work.
- Do not remove Shunter, LanceDB, SQLite, provider, or orchestration code in
  this task. In-place evolution preserves working code until its owning
  migration task replaces it.
- Do not stand up PostgreSQL/pgvector, add migrations or `sqlc`, define the
  configuration schema or threat model, or redesign CI. Those are separate
  bounded tasks.
- Do not reopen or renumber the accepted ADRs.

## Acceptance criteria

- `docs/architecture/repository-build-baseline.md` explicitly chooses in-place
  evolution and explains what that preserves and what later tasks may replace.
- The document records the module, Go source floor, CLI entry point, exact
  build/test/inspection commands, existing CI authority, and just-in-time
  directory rule without contradicting the canonical specification or ADRs.
- `README.md` links the baseline document once from its Documentation section.
- The documented build and full-test commands pass unchanged.
- No wrapper, empty scaffold, dependency, runtime behavior, or CI change is
  introduced.
- The diff contains only the baseline document, its README link, and the
  harness-owned metadata transition for this task.

## Verification

Run:

```bash
go build ./...
go test ./...
go run ./cmd/revolvr --help
git diff --check
```

Then manually compare the baseline document against the accepted ADRs and the
specified Sections 8, 29, and 33. Confirm that it records current authority
without presenting the suggested future directory tree as already implemented.

## Completed provenance

- Implementation commit:
  `eef26865bbc89b37ddf53a6fbc1726b7bcfa87b6` (`Document repository build baseline`).
- Baseline-version follow-up:
  `7e9e6a574c96273eb8ebbee60b0910c0be05128f` (`Upgrade Go baseline to 1.26.5`).

## Expected completion report

Report both provenance commits, the baseline document and README link, the
build/test/CLI results, and confirmation that no empty scaffolding or
architecture implementation was introduced by the baseline task.
