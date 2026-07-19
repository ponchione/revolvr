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
that exact source commit. Raw Git published only
`refs/heads/level1-v0.1.0-rc.4` at the candidate SHA, and push-triggered CI run
`29688941202` passed exactly all ten mandatory EXT-15 jobs on that SHA. A new
separate RC.4 exact-checkout Go 1.26.5 artifact-attestation workflow has now
passed local validation and remote execution. Workflow commit and attestation
ref are exact at `52c2db07a86677e67921bcbfbcbdf26397b47615`; dedicated run
`29690065853`, job `88201098277`, and artifact `8443312175` succeeded, and the
same push's ten-job CI run `29690065840` also succeeded. The controller
launchers and workflow commit are not candidate source. RC.4 is only the
sequential candidate label inside open backlog task `EXT-20`; it is not a
separate backlog task or external-use approval. The next bounded pass is fresh
no-model suite preparation using immutable RC.4 authority.

## RC.4 Remote Artifact Attestation Authority

- Workflow/main commit and exact attestation-ref readback:
  `52c2db07a86677e67921bcbfbcbdf26397b47615`.
- Candidate ref remains exact at source
  `2546913e38ec273f64417dece2f91df78fd42fc2`.
- Dedicated run `29690065853`: `completed` / `success`:
  `https://github.com/ponchione/revolvr/actions/runs/29690065853`.
- Sole job `88201098277`, `Rebuild and attest Level 1 RC.4 candidate`:
  `completed` / `success`:
  `https://github.com/ponchione/revolvr/actions/runs/29690065853/job/88201098277`.
- Sole retained artifact: ID `8443312175`, name
  `level1-v0.1.0-rc.4-attestation`, 70,214,949 bytes, digest
  `sha256:0a3567ec0fbc31aff65424790402f81a20df3f22c49659854993dcbeb1eb8fbc`,
  created `2026-07-19T14:05:56Z`, expires `2026-10-17T14:03:12Z`.
- Controller archive download was unavailable because the public endpoint
  returned HTTP 401 and no token was present. The successful job ran every
  exact hash, metadata, version, reproducibility, checksum, and remote-authority
  assertion before upload; no controller-side archive-byte comparison is
  claimed.
- Ordinary CI run `29690065840` also completed successfully with exactly ten
  successful jobs on the workflow commit.
- No live/nested model operation, suite, tag, release, external-use approval,
  or `EXT-20` completion occurred.

## RC.4 Local Artifact Attestation Workflow

- Workflow: `.github/workflows/level1-rc4-candidate-attestation.yml`.
- Locally reviewed workflow SHA-256:
  `340b82093d469e86e2e27e4729a51caa1da88f814017d6f6ab1bcabd89a56101`.
- Sole trigger: push to `level1-v0.1.0-rc.4-attestation`.
- Collision-free raw-Git ref reserved for later publication:
  `refs/heads/level1-v0.1.0-rc.4-attestation`.
- Checkout authority: exact candidate source
  `2546913e38ec273f64417dece2f91df78fd42fc2`, not trigger HEAD.
- Toolchain: exact Go 1.26.5 with action cache disabled. Each of two clean
  `--no-local` source clones has separate build and module caches.
- Targets: Linux, Darwin, and FreeBSD amd64 with CGO disabled, module-readonly
  mode, local toolchain, trimpath, explicit clean VCS metadata, empty build
  ID, and `main.version=0.1.0`.
- Exact pair hashes are Linux
  `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
  Darwin
  `042563f350b71ec8cd5be1b49fc9d948383caa28087c0a5689bd6eb12f3808ab`,
  and FreeBSD
  `128b9f8ced3038a51534da63b9d9ffbaa5ea7341e0ab8dd17102fba86084a8e6`.
- Exact upload name: `level1-v0.1.0-rc.4-attestation`. It retains both binary
  sets, hashes, build metadata, empty build IDs, exact per-binary version
  authority, Linux version output, reproducibility evidence, and the complete
  authority manifest.
- Local YAML, constants, embedded-shell syntax, complete detached-source
  execution, and retained-output checks passed. Retained verification root:
  `/tmp/revolvr-ext20-rc4-attestation.Y4TLEM`; its 29-file relative hash-stream
  digest is
  `1c08a35517d12e1993184143c43a97c753644d1f0dae68de1cbd2a59ee07e4b9`.
- The first full-shell harness call failed before building because its process
  began in controller `main`; the one harness repair changed only CWD to the
  detached candidate clone and reran the same embedded shell unchanged.
- Remote candidate/ref/tag/workflow/artifact namespace checks passed before
  construction. All ten sealed inventories, three historical workflow hashes,
  seven candidate/attestation refs including exact RC.4 candidate authority,
  and four retired-suite absences were preserved.
- No commit, push, remote workflow/ref/run/artifact, suite, live/nested model,
  tag, release, external-use approval, or `EXT-20` completion occurred.

## RC.4 Candidate Ref And Remote CI Authority

- Remote candidate ref: `refs/heads/level1-v0.1.0-rc.4`.
- Exact remote readback:
  `2546913e38ec273f64417dece2f91df78fd42fc2`.
- Push-triggered Actions run: `29688941202` — `completed` / `success` —
  `https://github.com/ponchione/revolvr/actions/runs/29688941202`.
- Run identity: event `push`, branch `level1-v0.1.0-rc.4`, head SHA
  `2546913e38ec273f64417dece2f91df78fd42fc2`.
- Exact successful jobs, all `completed` / `success` at that head SHA:
  - `88198118677` — Go 1.22 source floor and tests
  - `88198118646` — Production autonomous strict-fake suite
  - `88198118664` — Race tests
  - `88198118665` — Vet and module verification
  - `88198118653` — Fake-Codex success smoke
  - `88198118661` — Fake-Codex verification-failure smoke
  - `88198118641` — Build linux/amd64
  - `88198118668` — Build darwin/amd64
  - `88198118681` — Build freebsd/amd64
  - `88198118662` — Build Windows diagnostic stub
- Each job URL is
  `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/<job-id>`;
  `.agent/STATE.md` retains the expanded exact URLs.
- Post-CI checks preserved both RC.4 inventories, all eight historical sealed
  inventories, three historical workflows, six historical refs, and four
  retired-suite absences. The RC.4 attestation ref and every `*rc.4*` tag
  remain absent.
- No `main` commit/push, workflow, attestation ref, suite, live/nested model
  operation, tag, release, external-use approval, or `EXT-20` completion
  occurred.

## RC.4 Local Candidate Authority

- Candidate ID: `level1-v0.1.0-rc.4`.
- Release version: `0.1.0`.
- Exact source commit:
  `2546913e38ec273f64417dece2f91df78fd42fc2`.
- Exact source tree:
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`.
- Published authority: source is an ancestor of `origin/main` at
  `af123c7ce38e41982a2302d76cb7e2fa6bdf5608`.
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
- The exact candidate ref and source-floor remote CI authority are recorded
  above. Remote attestation authority is also recorded above and is complete;
  no suite, live/nested model, tag, release, or external-use authority exists
  yet. `EXT-20` remains open.

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

1. Start exactly one fresh pass with `agent-ext20-rc4-suite.sh` for the RC.4
   no-model suite-preparation gate.
2. Bind the guarded suite only to exact candidate source
   `2546913e38ec273f64417dece2f91df78fd42fc2`, Linux SHA-256
   `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
   and bundle
   `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec`, while
   preserving the settled release and exact Codex 0.144.4 authority.
3. Verify the shell, complete bundle, static suite, collectors, full Go suite,
   and diff; then prepare one new collision-free
   `/tmp/revolvr-ext20-rc4.XXXXXX/suite` with `--install-codex` but no model
   call. Inspect all prepared authority and the fail-closed confirmation gate.
4. Retain the prepared root and report the exact separately confirmation-gated
   live command without executing it. Keep `EXT-20` unchecked and external
   use unapproved.

The completed remote-CI pass did not create an attestation launcher. Do not
rerun `agent-ext20-rc4-remote.sh` because the candidate ref is now
intentionally nonempty.

Independent controller readback reconfirmed exact candidate, workflow, and
attestation refs, the successful dedicated run/job/artifact, and the ten-job CI
run through raw Git and public REST. Do not rerun the completed remote or
attestation launchers. The next bounded launcher is
`agent-ext20-rc4-suite.sh`.

Exact next command:

```bash
./agent-ext20-rc4-suite.sh
```

## Session Rules

- Read `AGENTS.md`, this file, `.agent/TASKS.md`, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and `.agent/LOOP_PROMPT.md` completely before acting.
- Use one new `codex exec` invocation per bounded task; never resume an old
  session.
- Do exactly one task per pass and preserve unrelated changes and immutable
  evidence.
- Never use `gh`.
- RC.4 candidate publication, remote CI, and artifact attestation are complete.
  The next launcher authorizes no-model suite preparation only; live-model,
  tag, release, and external-use authority remain excluded.
- The repository is durable memory; this handoff is only the resume pointer.
