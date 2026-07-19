# Agent Handoff

Updated: 2026-07-19

## Resume Point

The first unchecked backlog task remains `EXT-20`. RC.3 is immutable rejected
history, and its retained suite is permanently retired. The first local
Structured Outputs repair passed tests but failed independent audit because
not every object was strict-compatible and its regression guard was only a
finite denylist.

The follow-up repair for all four production model-facing schemas received a
separate read-only review, then explicit operator commit/push authority. It is
published on `main` as exact source commit
`2546913e38ec273f64417dece2f91df78fd42fc2` and tree
`8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`. `origin/main` readback matched
that commit after the raw-Git push. Focused, race, production-happy-path, and
full repository tests passed again immediately before publication without a
live model call.

Collision-free RC.4 now exists as a locally verified immutable candidate from
that exact source commit. The controller launcher `agent-ext20-rc4.sh` is not
candidate source. RC.4 is only the sequential candidate label inside open
backlog task `EXT-20`; it is not a separate backlog task or external-use
approval. The next bounded pass is independent verification followed, only
with explicit raw-Git publication authority, by the exact candidate-ref and
remote-CI gate.

## RC.4 Local Candidate Authority

- Candidate ID: `level1-v0.1.0-rc.4`.
- Release version: `0.1.0`.
- Exact source commit:
  `2546913e38ec273f64417dece2f91df78fd42fc2`.
- Exact source tree:
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`.
- Published authority: source is an ancestor of `origin/main` at
  `45a7f2384aaf21e36174660618c5f00a91edb1ab`.
- Candidate bundle:
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec/`.
- Candidate inventory SHA-256:
  `3535d7a2b46a0dbd3101428b4177e4c46baabc29190e5b1c580d90e6ff033f5d`.
- Build-instruction SHA-256:
  `5d87ff8eb5e89865729237dda500c8387ef5880b3c10ea0bd77f896938d606e9`.
- Verification bundle:
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec-verification/`.
- Verification inventory SHA-256:
  `75a2bcaba12d28d42a5012ad70995f4eb10363e250ec8028350e0802b0b8429c`.
- Artifact SHA-256 values:
  - Linux amd64:
    `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`
  - Darwin amd64:
    `042563f350b71ec8cd5be1b49fc9d948383caa28087c0a5689bd6eb12f3808ab`
  - FreeBSD amd64:
    `128b9f8ced3038a51534da63b9d9ffbaa5ea7341e0ab8dd17102fba86084a8e6`
- Construction used Go 1.22.12 for the source-floor suite and exact Go 1.26.5
  for release tests/builds. Two independent non-local clean clones produced
  byte-identical artifacts. Full tests, vet, module verification, vulnerability
  scans, metadata/version/build-ID checks, candidate/inventory self-checks,
  the Structured Outputs guard, and the production happy path passed.
- The Structured Outputs and happy-path results are local regression evidence
  only. No live API acceptance is claimed.
- Candidate-ref, workflow, artifact, bundle, run-root, and diagnostic collision
  checks passed before construction. The retained construction root is
  `/tmp/revolvr-ext20-rc4-build.okV2nU` and is not candidate authority.
- Historical preservation passed: eight available RC.1/RC.2/RC.3 inventories,
  20 content/layout targets, and six remote refs remained exact. The retired
  RC.3 suite path was absent throughout and was not recreated or used.
- The first independent metadata assertion contained a read-only `go tool nm`
  field-index error. Its single repair reran the full check without changing
  candidate bytes; the diagnostic is sealed in the verification bundle.
- Independent controller verification reran both complete inventories, the
  candidate self-verifier, artifact hashes and embedded identities, exact
  source publication/ancestry, candidate/ref/tag collisions, all eight
  historical inventories, all six historical remote refs, the focused schema
  and happy-path tests, and `go test -count=1 ./...`; all passed. The recorded
  local-candidate state was committed and pushed on `main` as
  `1917df5c374f8337a7bebb429478e7e16ea8420d`.
- No candidate ref, remote CI, attestation workflow, suite, live/nested model,
  tag, release, or external-use authority exists yet. `EXT-20` remains open.

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

The committed `internal/autonomouscycle/schema_compatibility_test.go` is the new
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

## Published Repair Scope

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
- `internal/autonomouscycle/schema_compatibility_test.go`
- `internal/autonomouscycle/worker_prompt.go`
- `internal/autonomousplanning/contracts_test.go`
- `internal/autonomousplanning/schema.go`
- `internal/supervisor/prompt_schema_test.go`
- `internal/supervisor/schema.go`

No dependency was added. The exact scope above was committed as
`2546913e38ec273f64417dece2f91df78fd42fc2` and pushed to raw-Git
`origin/main`. No candidate ref, tag, release, live/nested model call, or
external-use approval occurred.

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

1. Start one fresh session with `agent-ext20-rc4-remote.sh`. Running that
   launcher is explicit authority for only the collision-safe candidate-ref
   creation described below.
2. Raw Git must recheck that `refs/heads/level1-v0.1.0-rc.4` is absent
   immediately before creating it at exact source
   `2546913e38ec273f64417dece2f91df78fd42fc2`; it must not move any historical
   ref or use a later controller/state commit as candidate source.
3. Require the complete EXT-15 push-triggered CI matrix to pass on that exact
   candidate SHA and record the remote ref readback plus every job conclusion.
4. Stop after remote CI. The RC.4 exact-checkout Go 1.26.5 attestation workflow,
   no-model suite preparation, and any separately confirmed live model work are
   later bounded passes. Keep `EXT-20` unchecked and external use unapproved.

Exact next command:

```bash
./agent-ext20-rc4-remote.sh
```

## Session Rules

- Read `AGENTS.md`, this file, `.agent/TASKS.md`, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and `.agent/LOOP_PROMPT.md` completely before acting.
- Use one new `codex exec` invocation per bounded task; never resume an old
  session.
- Do exactly one task per pass and preserve unrelated changes and immutable
  evidence.
- Never use `gh`.
- The RC.4 remote launcher authorizes only collision-safe candidate-ref
  creation and exact-source remote CI evidence. It must remain isolated from
  later attestation, suite, live/nested model, tag, release, and external-use
  work.
- The repository is durable memory; this handoff is only the resume pointer.
