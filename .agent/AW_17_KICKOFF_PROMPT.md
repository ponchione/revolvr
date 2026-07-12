# Fresh AW-17 Structured Needs-Input Pass

You are starting one bounded autonomous-workflow task in a brand-new Codex
session. Do not rely on prior chat history and do not resume an older session.
The repository files and Git history are the durable source of truth.

## Read First

Read these files completely before editing:

- `AGENTS.md`
- `.agent/HANDOFF.md`
- `.agent/AUTONOMOUS_WORKFLOW_REFACTOR.md`
- `.agent/TASKS.md`
- `.agent/STATE.md`
- `.agent/DECISIONS.md`

Then inspect the current contracts and boundaries relevant to AW-17:

- `internal/autonomous/contracts.go`
- `internal/autonomous/state.go`
- `internal/autonomous/dossier.go`
- `internal/autonomouspolicy/policy.go`
- `internal/autonomousstate/store.go`
- `internal/autonomousstate/history.go`
- the planning, audit, attempt, and optional-role history/store files under
  `internal/autonomousstate`
- `internal/supervisor/schema.go`
- `internal/supervisor/prompt.go`
- `internal/supervisor/execution.go`
- `.agent/profiles/supervisor.md`

Inspect focused tests beside those files before choosing an API or persistence
shape. Use `rg` to find every existing `needs_input`, `NeedsInput`, terminal
action, state-transition, dossier, and history-reconstruction reference.

## Task

Do exactly one task: **AW-17 — add structured `needs_input` handling**.

Current backlog contract:

- Scope: extend autonomous contracts/state with a terminal-for-now supervisor
  outcome containing the exact question, blocking reason, mutually exclusive
  options, a recommendation, and independent work that may continue. Add
  answer/resume semantics without guessing a product decision.
- Acceptance: unanswered input prevents unsafe task actions; answers are
  durable and tied to the question/version; stale answers are rejected; clean
  needs-input tasks can yield the scheduler; CLI/TUI projections can consume
  the state later.
- Verification: add contract, persistence, stale-answer, resume, clean-yield,
  unsafe-state, and restart tests; run focused tests and `go test ./...`.

AW-17 has not been started. The current `NeedsInputDetail` in
`internal/autonomous/state.go` is deliberately minimal, and
`internal/autonomouspolicy` deliberately refuses routing from `needs_input`
until this task supplies explicit semantics. Replace that placeholder with a
complete, validated design; do not treat it as an already chosen data model.

## Required Behavior

### Structured supervisor outcome

Add a distinct validated terminal-for-now supervisor outcome. It must not be
encoded as free-form `block` rationale. The validated outcome and its durable
projection must express, at minimum:

- exact task identity;
- a stable question identity and positive version/revision;
- the exact user-facing question;
- the concrete reason the answer blocks safe progress;
- at least two distinct, mutually exclusive options with stable IDs and clear
  operator-facing meaning;
- one recommendation that references exactly one offered option and explains
  why it is recommended;
- typed evidence/input references supporting the question;
- a structured declaration of any independent work that can continue without
  the answer, including enough authority for Go policy to reject work that is
  actually dependent on the missing choice.

Question IDs, revisions, and content identities may be supplied through the
strict supervisor schema or deterministically assigned by the harness, but
they must never be accepted as unvalidated prose or allowed to drift between
the raw decision, persisted state, and answer operation.

Decide whether this is a new supervisor action or a rigorously discriminated
terminal outcome using the smallest shape consistent with the existing
`SupervisorDecision`, JSON schema, parser, and pure policy. Whichever shape is
chosen, validation must reject worker profiles, retry strategy, correction
authority, or unrelated success criteria on the terminal-for-now outcome.
Update the supervisor schema, prompt/profile instructions, parsing tests, and
decision validation together. Preserve deterministic JSON and compatibility
for every existing AW-01 through AW-16 decision.

Do not allow an arbitrary option label, option index, recommendation prose, or
model rationale to become control authority. Stable typed IDs and exact
validated references must carry control semantics.

### Durable question and answer lifecycle

Persist the complete question and its lifecycle in canonical AW-02 execution
state. An answer must be append-only durable evidence, not a transient CLI
argument and not an overwrite of the original question. It must bind exactly
to:

- task ID;
- question ID;
- question version/revision;
- one offered option ID;
- the exact question identity/content hash or equivalent deterministic
  identity;
- answer provenance and timestamp supplied by the harness/operator boundary.

Define explicit state operations for at least:

1. recording a validated supervisor needs-input outcome;
2. recording an operator answer to the current question;
3. resuming the task only from that exact answered question.

An unanswered current question keeps the task suspended. An answer for an old
question, old revision, different task, changed option set, or superseded
question must fail as stale rather than being guessed or silently retargeted.
Replay of the exact same operation must be idempotent; reuse of an operation ID
with different material must fail as a conflict. Questions and answers must
remain visible after resume and restart so later dossiers and operator
projections can explain the decision.

Do not make the raw task Markdown a model-authored scratchpad. Keep canonical
runtime state and immutable transition evidence under the existing
`.revolvr/autonomous/tasks/<task-id>/` namespace.

### Persistence and crash safety

Follow the existing autonomous-state persistence invariants rather than adding
an unrelated storage mechanism:

- validate the canonical task and safe paths;
- share the task's existing `state.lock`;
- use exact state hash/byte-size compare-and-swap;
- create immutable, versioned question/answer/resume history before canonical
  state replacement;
- use synchronized temporary writes, atomic rename, directory sync, and strict
  readback;
- make replay, stale-writer behavior, operation conflicts, and pre/post-rename
  crash recovery explicit;
- teach committed-history reconstruction to traverse the new evidence-only
  transitions so planning/audit/attempt/optional-role authority is not hidden.

Use a focused package boundary if that matches the existing
`autonomousplanapply`, `autonomousauditapply`, `autonomousattempt`, and
`autonomousoptional` patterns. Keep the domain contracts in
`internal/autonomous` and persistence primitives in `internal/autonomousstate`.

### Safety, policy, and scheduler yield

`needs_input` is terminal for the current unattended decision, but it is not a
completed/cancelled/blocked task and it must be resumable through the explicit
answer operation.

Go policy, not supervisor prose, must decide whether suspension is clean enough
to yield future scheduling. At minimum, refuse a clean yield or resume when
source state is unsafe/unknown, a source-mutating operation is in flight, the
question is malformed/stale, or durable evidence does not match the current
task/state/source. Do not hide dirty or ambiguous Git state behind
`needs_input`; surface a typed unsafe result that prevents a scheduler from
moving on.

Unanswered questions must prohibit actions whose correctness depends on the
answer. If independent work is represented as potentially routable, only
exactly declared, policy-validated, non-conflicting work may be considered; do
not infer independence from prose. It is acceptable for AW-17 to expose a pure
future-scheduler gate/projection rather than build a scheduler. AW-22 through
AW-24 own terminal loops and queue scheduling.

After an exact durable answer, resume to the appropriate nonterminal lifecycle
only through an explicit validated transition. Do not auto-select the
recommended option, auto-run a worker, start another supervisor session, or
consume an AW-15 action attempt merely for recording input/resume.

### Dossier and future UI projections

Render the current question, options, recommendation, blocking reason,
independent-work declarations, answer status/provenance, and prior resolved
questions deterministically in the dossier. Keep enough typed public state for
later app/CLI/TUI work to project without reparsing prose, but do not add
operator-facing CLI or TUI commands in AW-17; those are owned by AW-27/AW-28.

## Compatibility and Scope Boundaries

Preserve all established AW-01 through AW-16 behavior and these package
boundaries:

- `autonomouspolicy.Evaluate` remains pure.
- `autonomouscycle.Run` performs one supervisor decision and at most one
  worker; a needs-input decision starts no worker, verification, audit, or
  commit.
- `autonomousattempt` remains one-operation attempt/budget authority. Recording
  a question, answer, or resume does not invent a model attempt or reset
  budgets.
- `autonomouscorrection` remains one bounded correction/final-verification/
  re-audit sequence.
- `autonomousoptional` remains one conditional role operation; it does not
  answer questions or impose role ordering.
- Verification remains evidence-only; audit/finding persistence stays in its
  current packages.
- `internal/passpolicy` remains exclusively `mixed-pass-v1`.
- Do not wire an autonomous CLI, TUI, `runonce`, repeated supervisor loop,
  queue scheduler, notification, archive, worktree, or security-sandbox path.
- AW-18 owns per-task worktree isolation and checkpoint recovery.
- Do not add dependencies.
- Do not start a nested Codex run or invoke a live model.
- Do not commit; the operator will decide when to commit this one-task pass.

If backward compatibility with the old minimal `{"reason": ...}`
`NeedsInputDetail` requires an explicit legacy read path, keep it narrow and
fail closed for answer/resume authority. Do not silently fabricate option IDs,
question versions, recommendations, or answers from legacy prose.

## Tests

Add focused, table-driven tests that cover at least:

- deterministic JSON round trips and schema output;
- valid needs-input decisions and every malformed field/composition;
- duplicate/invalid option IDs, fewer than two options, ambiguous options, and
  recommendations outside the option set;
- exact independent-work validation and dependent/unsafe route rejection;
- entering `needs_input` without starting a worker, verification, audit, or
  commit;
- unanswered routing refusal;
- answer persistence tied to exact task/question/version/identity/option;
- stale task, stale state, stale version, changed options, unknown option, and
  superseded-question answers;
- exact replay versus operation-ID conflict;
- explicit resume, double resume, answer disappearance/rewrite rejection, and
  preserved budget/plan/acceptance/finding/audit/optional-role evidence;
- clean yield versus dirty, unknown, in-flight, or mismatched source state;
- restart/reopen and pre/post-state-write failure recovery;
- immutable history traversal through question/answer/resume transitions;
- deterministic dossier rendering for unanswered, answered, resumed, and
  legacy states;
- existing supervisor actions, mixed-pass behavior, and the complete suite.

Use real temporary Git repositories only where source-safety behavior requires
them. Use fake/injected runners; no live Codex call is required.

## Verification

Format every changed Go file, then run the smallest focused packages first.
At minimum, finish with:

```bash
gofmt -w <changed-go-files>
go test -count=1 ./internal/autonomous ./internal/autonomouspolicy \
  ./internal/autonomousstate ./internal/supervisor
go test -count=1 ./...
go vet ./<changed-packages>
git diff --check
```

If a focused package list changes with the implementation, include every
directly affected package, including any new needs-input application package.
Do not mark AW-17 complete unless all required verification passes.

## Durable State and Stop Condition

Before stopping:

1. Mark only AW-17 complete in `.agent/TASKS.md` if acceptance and verification
   pass.
2. Add a precise AW-17 completion record to `.agent/STATE.md` with contracts,
   persistence semantics, safety/yield behavior, files changed, tests, exact
   commands, results, remaining scope, and blockers.
3. Add only implementation decisions actually embodied in code to
   `.agent/DECISIONS.md`.
4. Refresh `.agent/HANDOFF.md` so AW-18 is clearly the next unchecked task.
5. Remove this consumed kickoff prompt after the AW-17 handoff is complete.

Stop after AW-17. Do not begin worktree isolation, safety preflight,
finalization, CLI/TUI projection, or scheduler work.

End with:

- Task selected
- Files changed
- Verification run
- Result
- Suggested next task
