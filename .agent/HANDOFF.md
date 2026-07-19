# Agent Handoff

Updated: 2026-07-18

## Resume Point

The first unchecked backlog task remains `EXT-20`. RC.3 is immutable rejected
history, and its retained suite is permanently retired. The first local
Structured Outputs repair passed tests but failed independent audit because
not every object was strict-compatible and its regression guard was only a
finite denylist.

The current uncommitted working tree, based on exact `main` commit
`45e92302843ad1cafe7a4a6bc58a319d606fb497`, contains the follow-up repair for
all four production model-facing schemas. The follow-up implementation and its
complete dirty-tree scope have now received a separate read-only review. The
four generated production schemas satisfy the documented structural rules
that caused the live RC.3 rejection and the failed first-repair audit. Focused,
race, production-happy-path, and full repository tests pass without a live
model call.

Work is paused for the day. Do not start another speculative schema-repair
loop. The next operator decision is whether to authorize committing and
pushing this already-verified repair. No new release candidate exists. If a
later pass creates one from the committed repair, `RC.4` is only its sequential
candidate label; it is not a separate backlog task or completed artifact.

## RC.3 Rejection And Preservation Authority

- Rejected candidate source:
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`.
- Failed suite: `/tmp/revolvr-ext20-rc3.Qghf19/suite`.
- Failed operation: `ext20-802d9db69596-01`.
- Codex run: `019f761f-078d-7b81-932b-278339f2a000`.
- Evidence bundle:
  `/tmp/revolvr-ext20-rc3.Qghf19/suite/evidence/repo-a/01-successful-source-change-1`.
- API result: HTTP 400 `invalid_json_schema` because
  `properties.conflicts.uniqueItems` is not permitted.
- Terminal result: `unsafe_or_ambiguous`, zero worker attempts, zero
  verification, zero audits, zero commits, and unchanged source trees.

Both evidence checksum manifests passed before editing and after all code
checks. Read-only content/layout fingerprints matched exactly:

- evidence bundle:
  `e47642eb4e8ade29ff213a3012891dc11a4bf800b654f80cb8c08a527564c689` /
  `bded4ce56ff6b2d8407978a40a3945b06eb1c0e982ec942e3671b14258b1b335`
- whole suite:
  `e070947f3a6cc3d0f598a3a78948757d7c1c0837c8baad70028cffb54b5734be` /
  `84e24f06525d81af2ff84061488d19170cc9791ca3139632892e3c5bf0431d58`

Never retry, reconcile, relabel, reuse, or mutate the RC.3 suite or bundle.
RC.1 and RC.2 remain immutable rejected history too.

## Failed First Repair Audit And Follow-up

The failed audit established these mandatory rules:

- every object has concrete `properties` and
  `additionalProperties: false`;
- every declared property appears exactly once in `required`;
- semantic optionals use supported required-null or required-empty forms;
- unconstrained objects are prohibited;
- compatibility uses an explicit supported-subset allowlist, not a denylist.

The follow-up covers all ordinary production builders:

- `supervisor.DecisionOutputSchema`
- `autonomousplanning.PlanningOutputSchema`
- `autonomousaudit.AuditOutputSchema`
- `autonomous.CorrectionOutputSchema`

Every generated object and definition is closed, every property is required,
every array has concrete `items`, and nullable values retain the existing Go
zero/nil contract. Strict decoding and Go validators retain action/profile,
needs-input, child-task, finding authority, correction partition, outcome,
evidence, and duplicate-rejection authority.

Audit provenance now exposes a closed model projection with
`verification.summary.tiered` fixed to null. The apply boundary compares that
projection and deterministically reattaches the exact trusted full tiered
verification result before canonicalization and persistence. The smaller
optional final-gate projection remains exact and closed.

The untracked `internal/autonomouscycle/schema_compatibility_test.go` is the new
stdlib-only recursive guard. It enumerates all four builders, uses an explicit
keyword allowlist, distinguishes property/`$defs` names from keywords,
validates nullable objects, arrays, local refs, and all definitions, and
reports exact JSON paths. It has negative coverage for the malformed fixtures
and unsupported keywords required by the failed audit.

Independent review noted that this regression helper does not itself enforce
the documented root-level `anyOf` prohibition or quantitative schema limits
(property count, nesting depth, aggregate schema-string size, and enum size).
The current four production schemas do not use a root `anyOf` and are plainly
below those limits, so this is test-hardening scope rather than evidence that
the current repair is API-incompatible. Do not turn this observation into
another repair loop unless the operator explicitly chooses to expand the
guard before the next candidate.

## Dirty Tree Scope

- `.agent/DECISIONS.md`
- `.agent/HANDOFF.md`
- `.agent/STATE.md`
- `internal/autonomous/contracts_test.go`
- `internal/autonomous/correction.go`
- `internal/autonomous/correction_test.go`
- `internal/autonomous/needs_input_test.go`
- `internal/autonomousaudit/contracts.go`
- `internal/autonomousaudit/contracts_test.go`
- `internal/autonomousaudit/schema.go`
- `internal/autonomousauditapply/apply.go`
- `internal/autonomousauditapply/apply_test.go`
- `internal/autonomouscycle/schema_compatibility_test.go` (untracked)
- `internal/autonomouscycle/worker_prompt.go`
- `internal/autonomousplanning/contracts_test.go`
- `internal/autonomousplanning/schema.go`
- `internal/supervisor/prompt_schema_test.go`
- `internal/supervisor/schema.go`

No dependency was added. No commit, push, tag, branch/ref publication, release,
live/nested model call, or external-use approval occurred.

## Local Verification

Passed:

```bash
gofmt -w <every changed Go file>
go test -count=1 ./internal/autonomous ./internal/supervisor ./internal/autonomousplanning ./internal/autonomousaudit ./internal/autonomouscycle ./internal/autonomousauditapply
go test -race -count=1 ./internal/autonomous ./internal/supervisor ./internal/autonomousplanning ./internal/autonomousaudit ./internal/autonomouscycle ./internal/autonomousauditapply
go test -count=1 ./internal/app -run '^TestProductionAutonomousHappyPath$'
go test -count=1 ./...
go test -count=1 ./internal/autonomouscycle -run '^TestProductionModelOutputSchemasUseSupportedStructuredOutputsSubset$'
git diff --check
```

Direct inspection of generated schema bytes found zero structural violations
and only allowlisted keywords. Generated SHA-256 values:

- supervisor:
  `5ef89243892e156bfdf098c132ea42ddc0a0ff74bd12af5276a493ac16be6c76`
- planning:
  `b4d088d7833a604ae4e91999dbc52a49b925d75ecfb3953dc56bdcceca2a1e09`
- audit:
  `899b551851549974b07c5352ba675e11f598763433c31c458a4c21e1e37e5eb3`
- correction:
  `0badb90760b7ca013c2cc7cb0ebf4c54f404a9d749dd9f5d29e4051e5812f022`

The final independent read-only pass also reran formatting inspection, the
focused race suite, the production autonomous happy path, the complete
repository suite, `git diff --check`, and both retained RC.3 checksum
manifests; all passed. The audit prompt/provenance projection and trusted
tiered-verification reattachment path were reviewed without a blocking
finding.

These are local compatibility and regression results only. No live API call was
made, so no API-acceptance claim is authorized.

## Next Ordered Work

1. Start a fresh session and read the durable state plus the complete dirty
   tree. Confirm the base remains `45e92302843ad1cafe7a4a6bc58a319d606fb497`
   and that no other terminal changed the candidate.
2. Wait for explicit operator authority before committing or pushing the
   verified repair. Do not require another audit merely because a new session
   starts.
3. Keep `EXT-20` unchecked and external use unapproved. Local tests do not
   replace the required fresh live/API acceptance pass.
4. Only after the repair is committed and pushed may a separately bounded pass
   construct the next immutable candidate. That candidate would be labeled
   RC.4. Never reuse RC.3's suite, evidence, operation, run, ref, workflow,
   artifact, or diagnostic.

Exact next read-only command:

```bash
GIT_OPTIONAL_LOCKS=0 git status --short && git diff --check
```

## Session Rules

- Read `AGENTS.md`, this file, `.agent/TASKS.md`, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and `.agent/LOOP_PROMPT.md` completely before acting.
- Use one new `codex exec` invocation per bounded task; never resume an old
  session.
- Do exactly one task per pass and preserve unrelated changes and immutable
  evidence.
- Never use `gh`.
- Do not commit, push, tag, publish refs, release, construct RC.4, or run a
  live/nested model without exact separate authority.
- The repository is durable memory; this handoff is only the resume pointer.
