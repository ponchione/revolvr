# Agent State

## Current Focus

The audit backlog remains closed. A new working release gate now lives at
`.agent/AUTONOMOUS_EXTERNAL_READINESS.md` for autonomous use in external
projects. Its previously open policy questions are settled in that document
and `.agent/DECISIONS.md`: staged readiness levels, quarantine rather than
heuristic in-flight resume, rootless Linux OCI isolation for unattended modes,
mode-aware preflight, exact Codex executable/version authority, finite
unattended budgets, sequential first queue approval, immutable tagged release
authority, and quantitative dogfood/soak thresholds.

The settled gates are now decomposed into 41 ordered, independently verifiable
.agent/TASKS.md items. The sequence approves attended single-task operation
first, then a Linux-only sequential bounded queue, then the Linux-only
foreground daemon. Every item has explicit acceptance and verification
evidence, and release commit/push/tag actions retain the repository rule that
they require direct operator authorization.

EXT-01 through EXT-19 are complete. EXT-14's previously implemented production
interruption matrix has now passed its separate fresh verification pass.
Doctor, status, canonical task loading, configuration reads, and exact-task/
queue admission use one descriptor-backed, read-only repository-path
inspection. Doctor now normalizes bare invocation to
attended-task, supports the three settled modes, validates the strict canonical
autonomous graph and protected task/state/archive authority, and optionally
requires one exact attended task to be ready. Preflight and execution now share
the initial Git repository, submodule, cleanliness, platform, verification, and
finite attended-bound admission. External admission now binds the exact
release-authored Codex version and resolved executable digest plus the resolved
Git executable identity, rechecks them before execution, and records them in
effective configuration and invocation provenance. A standalone strict fake
Codex executable now validates the complete invocation contract and produces
deterministic supervisor/worker evidence through the ordinary runner. The real
production task runner now has exact strict-fake composition proofs spanning
the direct happy path, a blocking finding through one cited correction and
clean re-audit, and the complete attended terminal matrix: needs input,
authorized block, verification failure, no progress, trusted safety refusal,
caller cancellation, exact durable replay, and maximum cycle. Exact
task-workspace proof now binds the deterministic task branch,
baseline, control root, Git common directory, linked-worktree registration,
ownership marker, current HEAD, and checkpoint evidence while refusing
foreign or drifted authority before source mutation.
The shared commit boundary now uses the same exact literal admitted path set
for staging and `git commit --only`, so late unrelated index/worktree bytes
cannot enter a generated commit. The unused destructive workspace restore
surface is removed; production composition is command-spy proven free of
push, merge, rebase, reset, clean, and stash while an unrelated linked
worktree remains unchanged. The real-Git containment matrix now proves dirty
and staged admission refusal, ignored-source refusal before workspace
publication, active-submodule refusal, exact SHA-1 and SHA-256 task commits, a
linked control worktree, and an operator commit injected during task
publication. Exact
control/task/unrelated branch, index, worktree, commit, and sentinel authority
remains separated. The external recovery contract now enumerates all 30
before/during/after transition seams for supervisor, worker, verification,
commit, checkpoint, audit, finalization, queue reconciliation, notification,
and archive publication. Every row binds exact durable replay, quarantine,
readiness-level continuation, prohibited inference, and operator action.
The next fresh task remains EXT-20. RC.1 and its remote evidence are immutable
rejected history after the omitted-work-directory defect. RC.2 and its fresh
exact-commit CI/artifact attestation are also immutable rejected history after
its first live operation exposed inconsistent source-lock default authority.
RC.3 source `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, candidate evidence,
remote CI, artifact attestation, failed suite, and retained live evidence are
now immutable rejected history: operation `ext20-802d9db69596-01` failed
before inference because the supervisor Structured Outputs schema emitted
unsupported `uniqueItems`. The suite is permanently retired. The first local
schema repair passed tests but failed independent audit because its objects
were not all strict-compatible and its regression guard was only a denylist.
The follow-up closes all four production schemas and adds a recursive
supported-subset guard. Its complete dirty-tree scope passed a separate
read-only review and was committed and pushed with explicit operator authority
as exact source `2546913e38ec273f64417dece2f91df78fd42fc2`, tree
`8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`. RC.4's candidate, exact ref,
ten-job CI run, artifact-attestation workflow/run/artifact, guarded suite, and
terminal failure are now immutable rejected history. Its first supervisor
selected `implement` from `pending` because the prompt omitted exact lifecycle
routing authority; runtime enforcement correctly rejected it before any
worker attempt. The bounded no-model source remediation passed independent
review and was published as exact source
`19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, tree
`2fb39c93694e72d986e7a8a849a542fc1bf1728d`. Collision-free RC.5 is now
constructed and locally verified from only that source. Its candidate and
verification bundles remain local; no candidate ref, remote CI, workflow,
suite, live model call, tag, release, external-use approval, or `EXT-20`
completion has occurred. Local tests do not establish live API acceptance.
Recovery inspection uses a distinct
read-only workspace/Git inspection path that takes no mutation lease and
publishes no retained ambiguity ref when live HEAD has drifted. EXT-14 now has
independent focused, race, and full-suite verification evidence.
Current external-project decision remains not approved; the readiness
document's remaining blockers stay open until their ordered tasks pass.

## EXT-20 RC.5 Replacement Candidate — Local Verification (2026-07-19)

- Task selected: construct and locally verify collision-free candidate
  `level1-v0.1.0-rc.5` from exact published source commit
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`. The source commit was clean,
  published, and reachable from `origin/main`; controller-only helper
  `agent-ext20-rc5.sh` was absent from the source and was not run or copied.
- Files changed: added the ignored settled build helper
  `.revolvr/release-candidates/build-level1-v0.1.0-rc.5.sh`, immutable candidate
  bundle `.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61`, and
  separate immutable verification bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61-verification`;
  updated this state file, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`.
  `.agent/TASKS.md` is unchanged and `EXT-20` remains unchecked. No product
  source, dependency, workflow, ref, tag, suite, or historical evidence changed.
- Collision and source authority passed before construction: local and remote
  RC.5 candidate/attestation refs, RC.5 tags, bundle paths, workflow, remote
  artifact name, run root, and diagnostic were absent. Exact Go authority was
  `/usr/local/go/bin/go` at `go1.26.5`; the source-floor toolchain resolved as
  `go1.22.12`.
- Verification passed from retained clean clone root
  `/tmp/revolvr-ext20-rc5-build.6Ci9vy/source-authority`: focused lifecycle
  routing, supervisor prompt/provenance, cycle fail-closed, Structured Outputs,
  production happy-path, and strict-fake regressions in ordinary and race
  modes; verbose focused proof; Go 1.22.12 and Go 1.26.5 full suites; `go vet
  ./...`; `go mod verify`; and reachable/imported vulnerability scans. There
  are zero reachable or imported-package vulnerabilities; the sole module-only
  result remains Windows-only, uncalled `GO-2026-5024` in
  `golang.org/x/sys@v0.30.0`, fixed in v0.44.0.
- The settled EXT-18 build procedure produced Linux, Darwin, and FreeBSD amd64
  binaries with Go 1.26.5, release version `0.1.0`, `CGO_ENABLED=0`, trimpath,
  VCS metadata, and empty build IDs. Two independent `git clone --no-local`
  builds were byte-identical. Independent inspection confirmed exact
  `main.version` symbol size 16, exactly one `0.1.0` string, version output
  `revolvr 0.1.0`, source revision, `vcs.modified=false`, tool, command path,
  target, and CGO authority.
- Artifact SHA-256 values are Linux
  `1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043`,
  Darwin
  `a0ba1e05f76d92c1d20577c897a37bc2b4a3252a4e0fb10ef9d736f25b07645d`,
  and FreeBSD
  `f9b6da20be9497c5eb772f7b40945fceedc064ecb6e081809c9510d71462e2d6`.
  Build-instructions SHA-256 is
  `69e0e533258b88b810db465935e66c49fd4e294fb745fc13998115dc8951dcb8`.
- The candidate has 15 files, 13 inventoried payload entries, inventory
  SHA-256
  `ba718e4bef733a370cff72570b96e3c2f0db0af4b9ad8eedc77db2c965ca0b88`,
  and inventory-seal SHA-256
  `8bf947efd3d7f6467d500f88278913c0bcf5dd922331e558d483176a777584ab`.
  The verification bundle has 44 files, 42 inventoried evidence entries,
  inventory SHA-256
  `e57353d8b929758b44d234458dfb2c3b4bae0cf347eccc206ba9424312a0e366`,
  and inventory-seal SHA-256
  `2cded484b787daa903ebf457f3f96bb9520af122bd48114300d78e543f39ccb8`.
- RC.1 through RC.4 candidate/evidence inventories reverified before and after
  construction. RC.4 suite `/tmp/revolvr-ext20-rc4.DGg1pW/suite`, operation
  `ext20-2bd21aea4f72-01`, both terminal collector bundles, all historical
  remote refs, and all sentinels remained unchanged. The 40-target historical
  content/layout baseline SHA-256 is
  `b0adbd4c9082ca10a9c344bc0f1cdc24458a23da77db274b98bd27e5af6c38b2`;
  post-construction content/ctime and sealed inventories passed. The one
  evidence-harness stop was repaired by normalizing remote-ref order before
  comparison; ref identities did not change and no candidate authority changed.
- Result: local RC.5 candidate construction passed with no blockers. What
  remains is an independent read-only review, then separately authorized raw-
  Git publication of the still-absent `refs/heads/level1-v0.1.0-rc.5` at the
  exact source commit and remote CI on that exact ref. No remote or live-model
  acceptance is claimed.

## EXT-20 Lifecycle-Authority Controller Review And Publication (2026-07-19)

- Independent controller review found the repair bounded to the lifecycle
  authority projection, supervisor prompt/provenance, cycle handoff, and focused
  tests. Runtime lifecycle enforcement remains fail-closed; pending still
  admits only `plan`, `block`, and `needs_input`, and the model receives that
  exact ordered authority before deciding. No Structured Outputs, downstream
  action/profile, attempts, verification, audit, source-lock, or commit gate was
  weakened.
- Controller verification passed formatting; focused and race tests for
  `internal/autonomouspolicy`, `internal/supervisor`, and
  `internal/autonomouscycle`; production `TestProductionAutonomousHappyPath`
  and `TestStrictFakeCodexContract` in ordinary and race modes; `go test
  -count=1 ./...`; and `git diff --check`.
- RC.4 terminal preservation passed again: collector manifest, all 112
  inventoried files, evidence and whole-suite fingerprints, both candidate
  inventories, exact remote candidate/attestation refs, clean repository
  heads, and sentinels remained unchanged.
- Raw Git committed the reviewed repair and local state as
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7` (`Expose lifecycle routing
  authority to supervisor`), tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`, after a fresh `origin/main`
  parent check, then pushed it to `main`. This exact repair commit is the only
  allowed RC.5 source; the later `agent-ext20-rc5.sh` launcher/state commit is
  controller authority only.
- No candidate, ref, workflow, remote CI, attestation, suite, live/nested model
  operation, tag, release, external-use approval, or `EXT-20` completion
  occurred. The next bounded pass is collision-free local RC.5 construction.

## EXT-20 RC.4 Lifecycle-Authority Remediation (2026-07-19)

- Task selected: the bounded no-model lifecycle-authority remediation within
  unchecked `EXT-20`. RC.4 operation `ext20-2bd21aea4f72-01` remains
  `unsafe_or_ambiguous`: one supervisor decision selected `implement` from a
  `pending` lifecycle with no plan, while zero worker attempts, verification,
  audits, corrections, commits, or source changes occurred. The runtime gate
  remains unchanged in authority and still rejects `implement` from pending.
- Implementation: `internal/autonomouspolicy` now owns
  `autonomous-lifecycle-routing-authority-v1`, the single deterministic
  lifecycle-to-actions projection used by enforcement, prompt construction,
  and provenance. Pending exposes only `plan`, `block`, and `needs_input`;
  ready exposes the complete settled action vocabulary; every in-flight,
  suspended, finalizing, terminal, or unknown lifecycle fails closed before a
  supervisor call. The supervisor prompt renders the exact authority after the
  global profile, supervisor provenance advances to
  `revolvr-supervisor-provenance-v2`, and the cycle checks exact retained
  authority before accepting the supervisor result. Structured Outputs,
  action/profile validation, lifecycle enforcement, and all downstream gates
  remain intact.
- Files changed: `internal/autonomouspolicy/policy.go`,
  `internal/autonomouspolicy/policy_test.go`, `internal/supervisor/prompt.go`,
  `internal/supervisor/prompt_schema_test.go`,
  `internal/supervisor/execution.go`,
  `internal/supervisor/execution_test.go`,
  `internal/autonomouscycle/cycle.go`,
  `internal/autonomouscycle/cycle_test.go`, this file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. No suite, candidate,
  workflow, dependency, configuration, release, or task-backlog content
  changed; `EXT-20` remains unchecked.
- Verification passed:
  - `gofmt -w` on all eight changed Go files;
  - focused lifecycle/prompt/provenance/cycle regressions across
    `./internal/autonomouspolicy`, `./internal/supervisor`, and
    `./internal/autonomouscycle`;
  - `go test -count=1 ./internal/autonomouspolicy ./internal/supervisor
    ./internal/autonomouscycle`;
  - `go test -race -count=1 ./internal/autonomouspolicy
    ./internal/supervisor ./internal/autonomouscycle`;
  - `go test -count=1 ./internal/app -run
    'Test(ProductionAutonomousHappyPath|StrictFakeCodexContract)$'` and the
    same command with `-race`;
  - `go test -count=1 ./...`;
  - `git diff --check`.
- RC.4 preservation passed before editing and after all code checks. The
  collector manifest and all 112 bundle files verify; manifest SHA-256 remains
  `33a6e800fdd32b0e5873f3c59b2d90d4d47d73ae93f6700acf572e88bbd85a23`,
  inventory SHA-256 remains
  `81028ea618dee019fb37b95e91ac0863d105b31426893b10e633798ecca5d43b`,
  evidence content remains
  `b253ebb96f8c6e7989db20fa820aa14fd12f323b910a73aaae039d4fa2fbdc9a`,
  whole-suite content remains
  `a44d88d7419db1d6b325daaf792dd775fe46523d63e42f9503289e0059b7c2e2`,
  and the fresh pre/post metadata/layout fingerprint remains
  `7e1d80bb37022da5874c01db09cf07cefb6e39b86941117f75efc8dc69d0b722`.
  Candidate and verification inventory SHA-256 values remain
  `3535d7a2b46a0dbd3101428b4177e4c46baabc29190e5b1c580d90e6ff033f5d`
  and
  `75a2bcaba12d28d42a5012ad70995f4eb10363e250ec8028350e0802b0b8429c`;
  RC.4 refs and workflow hash remain exact. Both disposable repositories stay
  clean at their recorded heads and the sentinel comparison remains exact.
- Result: the bounded repair and its independent publication gate are complete
  with no blocker. Exact published commit/tree authority is recorded above.
  A fresh collision-free local RC.5 candidate may now be constructed, but it
  must not reuse RC.4 suite, operation, evidence, refs, workflows, artifacts,
  identities, or failed authority.

## EXT-20 RC.4 Terminal Live Failure And Root Cause (2026-07-19)

- Exact suite `/tmp/revolvr-ext20-rc4.DGg1pW/suite` entered its first guarded
  live operation and stopped immediately. Operation
  `ext20-2bd21aea4f72-01`, task `ext20-success-a1`, scenario
  `successful-source-change-1`, expected `completed`, observed
  `unsafe_or_ambiguous`, exit status 1. Terminal bundle:
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite/evidence/repo-a/01-successful-source-change-1`.
- Supervisor run `019f7afa-562a-732e-948a-920096198000` and Codex thread
  `019f7afa-5a6e-7c11-8f05-4e1ec8541a3e` produced a structurally valid
  `implement` / `implementer` decision with a bounded one-file strategy. The
  durable task lifecycle was `pending` with no plan. Runtime policy correctly
  failed closed: `pending lifecycle admits only "plan", "block", or
  "needs_input", not "implement"`.
- Root cause: the supervisor profile lists every global action and the dossier
  shows `Lifecycle: pending`, but the model-facing prompt does not state exact
  lifecycle-admitted actions. The model therefore lacked authority necessary
  to distinguish a reasonable implementation recommendation from an illegal
  route. No model coercion, retry, policy weakening, or dogfood exception is
  authorized; lifecycle authority must be communicated from a deterministic
  source shared with enforcement.
- Independent verification passed the collector's `--verify-manifest`, the
  bundle checksum and all 112 inventoried files, ledger export/replay evidence,
  exact candidate/Codex/config identities, and terminal operation/state JSON.
  Manifest SHA-256 is
  `33a6e800fdd32b0e5873f3c59b2d90d4d47d73ae93f6700acf572e88bbd85a23`;
  `files.sha256` and recorded bundle SHA-256 are
  `81028ea618dee019fb37b95e91ac0863d105b31426893b10e633798ecca5d43b`;
  evidence content fingerprint is
  `b253ebb96f8c6e7989db20fa820aa14fd12f323b910a73aaae039d4fa2fbdc9a`;
  whole-suite content fingerprint is
  `a44d88d7419db1d6b325daaf792dd775fe46523d63e42f9503289e0059b7c2e2`.
- Exactly one collector manifest and no aggregate exist. The supervisor
  completed once; worker attempts, verification, audits, corrections, commits,
  and source changes are all zero. Control repo-a and its retained workspace
  remain clean at exact head
  `a75d4f059721ec7c9320650bd49d6d4cef9526cf`; repo-b remains clean and
  untouched at `11eb46ae242cf2a3cb5ce32cf94e0df3aab2ab0b`; before/after outside
  sentinel evidence is identical.
- RC.4 and this suite are immutable rejected history and must never be retried,
  reconciled, relabeled, mutated, or reused. `EXT-20` remains unchecked. The
  bounded no-model source repair is recorded above and its launcher must not
  be rerun. Independent review and separately authorized publication are next;
  RC.5 construction remains a later collision-free gate.

## EXT-20 RC.4 First Confirmed Live No-Start Diagnostic (2026-07-19)

- After the operator returned from the first confirmation-gated launcher, the
  controller found no repository diff and no live-suite activity. Exact root
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite` still contains zero operation
  manifests, zero collector manifests, an empty aggregate, and only its two
  unchanged preparation logs. Both disposable repositories remain clean on
  `main` at exact prepared heads; no evidence, operation, Codex run, source
  change, or terminal suite result exists.
- Read-only diagnosis reconfirmed the exact prepared regular-file content
  fingerprint
  `5e988363634a5aa4739c3b4bfccce865d2cf6e2c7ddb634aaa4eb25750641305`,
  authority and plan, candidate/Codex identities, repository heads, sentinels,
  and zero-runtime state. No root mutation or model retry occurred.
- Because the orchestration wrapper retained no failure diagnostic, it is
  retired. The replacement `agent-ext20-rc4-live-direct.sh` requires a new
  exact confirmation argument, performs deterministic raw-Git, bundle,
  content, identity, repository, sentinel, and zero-runtime checks, and then
  directly executes the guarded suite once. Any preflight drift stops before
  live work. Any later suite failure or interruption remains terminal.
- `EXT-20` remains unchecked. No tag, release, or external-use approval exists.

## EXT-20 RC.4 Prepared-Suite Controller Verification (2026-07-19)

- Independent controller inspection confirmed the implementation diff changes
  exactly the RC.3-to-RC.4 source commit, Linux hash, and bundle constants in
  `scripts/dogfood-external-level1-suite.sh`; the release, Codex, plan,
  scenarios, thresholds, configuration, and confirmation gate did not change.
- Verification reran `bash -n` for the suite and collector, the complete RC.4
  bundle verifier, suite `--static`, `go test -count=1 ./...`, and
  `git diff --check`; all passed without a model call. Read-only retained-root
  checks reconfirmed `prepared.sha256`, authority/plan hashes, candidate hash
  and version, Codex hash and version, exact clean repo-a/repo-b heads, zero
  operation and collector manifests, and an empty aggregate.
- Raw Git fetched and reverified `origin/main`, exact RC.4 candidate ref
  `2546913e38ec273f64417dece2f91df78fd42fc2`, and exact attestation ref
  `52c2db07a86677e67921bcbfbcbdf26397b47615`. The suite-authority and durable
  changes were committed as
  `3284971acfc542fa64d600f7c40a58891b16cb7c` (`Bind Level 1 suite to RC.4
  candidate`) and pushed to `main` after a fresh remote-parent check.
- No model call, live operation, evidence mutation, retry, tag, release, or
  external-use approval occurred. The next separately confirmed pass must use
  `agent-ext20-rc4-live.sh` with its exact confirmation argument. `EXT-20`
  remains unchecked until live and retained-evidence verification pass.

## EXT-20 RC.4 Guarded No-Model Suite Preparation (2026-07-19)

- Task selected: the bounded RC.4 no-model suite-preparation subtask of
  `EXT-20`. The guarded suite now binds exact source
  `2546913e38ec273f64417dece2f91df78fd42fc2`, Linux SHA-256
  `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
  and immutable bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec`. Release
  output remains `revolvr 0.1.0`; Codex authority remains exact package
  `@openai/codex@0.144.4`, output `codex-cli 0.144.4`, and SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  No plan, schema, scenario, threshold, configuration, confirmation value, Go
  source, or dependency changed.
- Files changed: `scripts/dogfood-external-level1-suite.sh`, this file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. The implementation change is
  exactly three shell constants. `.agent/TASKS.md` remains unchanged with
  `EXT-20` unchecked.
- Verification passed: `bash -n` for the suite and collector; the complete
  RC.4 candidate-bundle verifier; suite `--static`; two collector
  `--fixture-only` collections; both `--verify-manifest` checks; canonical
  manifest comparison after excluding only `collected_at_utc`; `go test
  -count=1 ./...`; and `git diff --check`. The retained collector fixture root
  is `/tmp/revolvr-ext20-rc4-collector.5SFvwa`; both raw manifest SHA-256
  values are
  `5169090fe855302da1fe70ca98535d7b66e4769c1af81ddec27affd9e9fc64e9`
  and the timestamp-excluded canonical SHA-256 is
  `01a47fd5ab7f7eb8e7def144ce7cbef17d5b306366cb2967896fb7b874e8452c`.
- A new parent was created with exact template
  `/tmp/revolvr-ext20-rc4.XXXXXX`. The retained prepared suite is
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite`; preparation used only `--prepare
  --run-root /tmp/revolvr-ext20-rc4.DGg1pW/suite --install-codex`, installed
  the pinned package, and started no model. Suite ID is
  `ext20-2bd21aea4f72`; `authority.tsv` SHA-256 is
  `4f9b653c9e62e5fc5932b219952bbe61fccd79d331ac2bd7fcf2c570035eacb7`
  and `operation-plan.tsv` SHA-256 is
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`.
- Independent read-only inspection verified the prepared-authority checksum;
  exact candidate path/hash/version/source and clean VCS metadata; exact
  Codex package/path/hash/version; effective schema v8; and source-writer
  authority `timeout=32m0s heartbeat_interval=10m40s required=32m0s`. The
  unchanged 11-row plan has ten unique ready tasks. Both tracked disposable
  repositories are clean on `main`: repo-a has six ready tasks at
  `a75d4f059721ec7c9320650bd49d6d4cef9526cf`; repo-b has four ready tasks at
  `11eb46ae242cf2a3cb5ce32cf94e0df3aab2ab0b`.
- There are zero runtime operation manifests, zero collector manifests, and
  zero aggregate entries. Inspection used `GIT_OPTIONAL_LOCKS=0`; complete
  regular-file content fingerprint
  `5e988363634a5aa4739c3b4bfccce865d2cf6e2c7ddb634aaa4eb25750641305`
  and layout/type/mode/owner/size/link/inode/mtime/ctime fingerprint
  `5e52e1be955403644fd33ee2b95c832896994305f95806ebac533ec93525244f`
  matched before and after. The empty-confirmation guard was exercised in
  isolation and its source order proves refusal before prepared-root reads;
  neither `--live` nor the confirmation value was passed to the suite.
- The first inspection harness stopped on an overly specific config-rendering
  assertion that expected the doctor-only `required` field on the config-check
  line. The one read-only repair asserted config check's exact timeout and
  heartbeat plus doctor's exact required window, then reran the complete
  fingerprinted inspection successfully; the prepared suite never changed.
- Preservation passed. All ten available RC.1/RC.2/RC.3/RC.4 sealed
  inventories, all four attestation-workflow hashes, and all eight raw remote
  candidate/attestation refs reverified exactly. No retired suite was present,
  recreated, reused, reconciled, or relabeled, and no historical ref, bundle,
  workflow, hash, artifact record, diagnostic, or available root changed.
- Verification result: passed. No `gh`, commit, push, tag, live or nested
  Codex/model operation, release, or external-use approval occurred. There are
  no blockers for this bounded preparation task. What remains is a separately
  confirmed live pass using exactly:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-rc4.DGg1pW/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. That command was recorded but not run;
  `EXT-20` remains unchecked.

## EXT-20 RC.4 Remote Artifact Attestation Result (2026-07-19)

- Independent controller review passed the workflow's PyYAML structure and
  exact trigger/job constants, extracted embedded-shell `bash -n`, workflow
  SHA-256
  `340b82093d469e86e2e27e4729a51caa1da88f814017d6f6ab1bcabd89a56101`,
  retained 29-file output shape, and all six sealed `SHA256SUMS` rows. The
  candidate ref remained exact and the attestation ref/tag namespace was
  empty immediately before publication.
- Raw Git committed the reviewed workflow and local state on `main` as
  `52c2db07a86677e67921bcbfbcbdf26397b47615` (`Add RC.4 artifact attestation
  workflow`) and pushed it after a fresh parent check. An empty-expected
  force-with-lease then created only
  `refs/heads/level1-v0.1.0-rc.4-attestation` at that workflow commit. Remote
  readback matched `main` and the new attestation ref at the workflow commit,
  while `refs/heads/level1-v0.1.0-rc.4` remained exact candidate source
  `2546913e38ec273f64417dece2f91df78fd42fc2`.
- Dedicated run `29690065853`, `Level 1 RC.4 candidate attestation`, event
  `push`, branch `level1-v0.1.0-rc.4-attestation`, head SHA
  `52c2db07a86677e67921bcbfbcbdf26397b47615`, completed with `success`:
  `https://github.com/ponchione/revolvr/actions/runs/29690065853`. Its sole job
  `88201098277`, `Rebuild and attest Level 1 RC.4 candidate`, completed with
  `success`:
  `https://github.com/ponchione/revolvr/actions/runs/29690065853/job/88201098277`.
- The run retained exactly one unexpired artifact: ID `8443312175`, name
  `level1-v0.1.0-rc.4-attestation`, size 70,214,949 bytes, digest
  `sha256:0a3567ec0fbc31aff65424790402f81a20df3f22c49659854993dcbeb1eb8fbc`,
  created/updated `2026-07-19T14:05:56Z`, expiring
  `2026-10-17T14:03:12Z`. Its archive endpoint is
  `https://api.github.com/repos/ponchione/revolvr/actions/artifacts/8443312175/zip`.
  Public unauthenticated download returned HTTP 401 and neither `GH_TOKEN` nor
  `GITHUB_TOKEN` was available, so controller-side archive comparison was not
  possible. The successful remote job itself performed the two clean rebuilds,
  exact sealed hash/identity assertions, pair comparisons, final checksum
  check, and authority readback before upload.
- The same push's ordinary CI run `29690065840` completed with `success` at the
  exact workflow commit:
  `https://github.com/ponchione/revolvr/actions/runs/29690065840`. Exactly ten
  jobs were returned and every job completed successfully: IDs `88201098140`,
  `88201098145`, `88201098159`, `88201098170`, `88201098176`, `88201098180`,
  `88201098187`, `88201098200`, `88201098201`, and `88201098223`.
- No candidate ref, source, bundle, historical workflow/ref/evidence, tag,
  release, retired suite, or external-use decision changed. No `gh` or
  live/nested model operation ran. `EXT-20` remains unchecked. RC.4's
  candidate-ref, remote-CI, and artifact-attestation prerequisites are now
  complete; the next bounded pass is fresh collision-free no-model suite
  preparation through `agent-ext20-rc4-suite.sh`.

## EXT-20 RC.4 Remote Artifact Attestation Workflow (2026-07-19)

- Task selected: the bounded RC.4 release-artifact attestation-workflow
  prerequisite of `EXT-20` only. `EXT-20` remains unchecked.
- New separate workflow
  `.github/workflows/level1-rc4-candidate-attestation.yml` triggers only on a
  push to `level1-v0.1.0-rc.4-attestation`. It checks out exact candidate
  source `2546913e38ec273f64417dece2f91df78fd42fc2` and tree
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`, not trigger HEAD, and requires
  the published candidate ref to remain at that source while the triggering
  attestation ref and complete RC.4 ref/tag namespace match exact authority.
  The locally reviewed workflow SHA-256 is
  `340b82093d469e86e2e27e4729a51caa1da88f814017d6f6ab1bcabd89a56101`.
- The workflow installs exact Go 1.26.5 with setup cache disabled, makes two
  independent clean `--no-local` clones, and gives each pass separate
  `GOCACHE` and `GOMODCACHE` paths. Both passes build Linux, Darwin, and
  FreeBSD amd64 with disabled CGO, local toolchain selection, module-readonly
  mode, `-trimpath`, explicit VCS metadata, an empty build ID, and exact
  `main.version=0.1.0`.
- Every artifact must expose exact Go version, command path, `gc` compiler,
  trimpath, target, CGO, Git source revision, and `vcs.modified=false`
  metadata; an empty retained build-ID result; exactly one 16-byte
  `main.version` data symbol; and exactly one `0.1.0` string. Both Linux
  copies must print exactly `revolvr 0.1.0`.
- Each pass must reproduce sealed SHA-256 values Linux
  `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
  Darwin
  `042563f350b71ec8cd5be1b49fc9d948383caa28087c0a5689bd6eb12f3808ab`,
  and FreeBSD
  `128b9f8ced3038a51534da63b9d9ffbaa5ea7341e0ab8dd17102fba86084a8e6`.
  Corresponding pass artifacts must also be byte-identical.
- One upload named `level1-v0.1.0-rc.4-attestation` retains both binary sets,
  all six build metadata files, empty build-ID records, per-binary version
  authority, both Linux version outputs, the six-row hash manifest, the
  three-pair reproducibility table, and an exact authority manifest naming the
  workflow path, ref/namespace, artifact, candidate ref/ID/source/tree,
  toolchain, environment, flags, passes, targets, and hashes.
- Collision checks passed before construction. Raw Git read `origin/main` and
  local `HEAD` at launcher commit
  `e6cd453931b5fa1c0261e984749425e4409b0bb0`; remote candidate ref
  `refs/heads/level1-v0.1.0-rc.4` remained exact at the candidate SHA; proposed
  ref `refs/heads/level1-v0.1.0-rc.4-attestation`, every RC.4 tag, and the new
  workflow path on both remote main and candidate source were absent. Public
  REST listed only the three historical RC.1/RC.2/RC.3 artifacts, so the
  exact RC.4 artifact name and namespace were absent.
- Verification passed: PyYAML `BaseLoader` structure and exact-constant
  assertions; extracted embedded-shell `bash -n`; workflow-only whitespace
  checks; and execution of the complete unmodified embedded shell under host
  Go 1.26.5 from a detached exact-source clone with a collision-safe local
  raw-Git ref fixture. The shell created two further clean non-local clones
  and independently downloaded/built through separate caches. Retained-output
  inspection found exactly 29 regular single-link files, six exact hash rows,
  three identical pairs, six empty build-ID records, six exact version
  authorities, two exact Linux outputs, and complete authority and
  reproducibility manifests. The retained verification root is
  `/tmp/revolvr-ext20-rc4-attestation.Y4TLEM`; the relative-file/hash stream
  digest is
  `1c08a35517d12e1993184143c43a97c753644d1f0dae68de1cbd2a59ee07e4b9`.
- The first complete-shell harness invocation correctly failed before any
  build because the harness started Bash in controller `main` rather than the
  detached candidate workspace. The single harness repair changed only the
  invocation working directory and reran the same extracted shell unchanged;
  the successful retained root above contains the complete result. No
  workflow repair or candidate byte change was required.
- Preservation passed. Historical workflow SHA-256 values remain RC.1
  `d1314182a0cffd78927e6a5cc688e370c42f3d17a4e4ffe426f647a384c40a41`,
  RC.2
  `4c96ec62a3757878926b62aee65ce8ba3ec6ac2148ac251622ab294109312c6d`,
  and RC.3
  `76b0b9c3e683a4df5fa4103588ea668459b00efa18eee4930baa81c440df4da6`.
  All ten available RC.1/RC.2/RC.3/RC.4 inventory manifests and contents
  reverified. All six historical remote refs remained exact, the RC.4
  candidate ref remained exact, and the four retired suite roots remained
  absent. No historical workflow, ref, bundle, artifact, hash, suite,
  operation, run, or diagnostic was reused or changed.
- Files changed: the new RC.4 workflow, this file, `.agent/DECISIONS.md`, and
  `.agent/HANDOFF.md`. `.agent/TASKS.md` remains unchanged with `EXT-20`
  unchecked. No Go source or dependency changed, so no Go test was required.
  No `gh`, commit, push, remote CI, suite preparation, live/nested Codex or
  model operation, candidate source/ref mutation, tag, release, or
  external-use approval occurred. Blockers for this bounded local task: none.
- Collision-free raw-Git ref reserved for the controller's later reviewed
  publication is `refs/heads/level1-v0.1.0-rc.4-attestation`. The controller
  must independently verify and commit this workflow, publish only that absent
  ref with an empty-expected lease while preserving the candidate ref, require
  the exact remote job to pass, and record workflow commit/ref, run/job
  IDs/URLs/status/conclusions plus artifact ID/name/size/digest/creation/expiry
  and retained-archive comparison. No-model suite preparation and live
  quantitative dogfood remain later gates.

## EXT-20 RC.4 Remote-CI Verification And Attestation Handoff (2026-07-19)

- Independent controller raw-Git and public-REST readback reconfirmed exact
  candidate ref `level1-v0.1.0-rc.4` at
  `2546913e38ec273f64417dece2f91df78fd42fc2`, run `29688941202` as
  `completed` / `success` for that push branch/SHA, and exactly ten distinct
  mandatory jobs as `completed` / `success` on the same SHA.
- Both RC.4 inventories, the candidate self-verifier, and all eight historical
  inventories passed again. The remote-CI state was committed as
  `8c0379aa3fb6824fb56d4f3c1180f4cc411ada2a` (`Record RC.4 remote candidate
  CI`) and pushed to raw-Git `origin/main`.
- `agent-ext20-rc4-attestation.sh` is the next controller-only launcher. It is
  limited to local construction and complete local verification of the
  collision-free exact-checkout Go 1.26.5 RC.4 attestation workflow. It grants
  no commit, push, remote workflow/ref, suite, live model, tag, release, or
  external-use authority.
- What remains: run `./agent-ext20-rc4-attestation.sh`, then independently
  verify and publish its reviewed workflow/ref in a later controller step.
  `EXT-20` remains unchecked.

## EXT-20 RC.4 Exact-Candidate Remote CI (2026-07-19)

- Task selected: the bounded RC.4 candidate-ref and remote-CI subtask of
  `EXT-20` only. `EXT-20` remains unchecked.
- Before publication, both sealed RC.4 inventories, the candidate
  self-verifier, candidate ID `level1-v0.1.0-rc.4`, release `0.1.0`, source
  commit `2546913e38ec273f64417dece2f91df78fd42fc2`, source tree
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`, and all three artifact hashes
  reverified exactly. Candidate inventory SHA-256 remained
  `3535d7a2b46a0dbd3101428b4177e4c46baabc29190e5b1c580d90e6ff033f5d`;
  verification inventory SHA-256 remained
  `75a2bcaba12d28d42a5012ad70995f4eb10363e250ec8028350e0802b0b8429c`.
- The immediate pre-publication raw-Git gate fetched `origin` without tags,
  proved the exact source object/tree and ancestry from fetched
  `origin/main` at `af123c7ce38e41982a2302d76cb7e2fa6bdf5608`, proved remote
  candidate and attestation refs absent, proved every remote tag matching
  `*rc.4*` absent, and reread all six historical RC.1/RC.2/RC.3 refs at their
  sealed identities.
- The sole external mutation used the required empty-expected lease:
  `git push --force-with-lease=refs/heads/level1-v0.1.0-rc.4: origin
  2546913e38ec273f64417dece2f91df78fd42fc2:refs/heads/level1-v0.1.0-rc.4`.
  Remote readback returned exactly
  `2546913e38ec273f64417dece2f91df78fd42fc2` at
  `refs/heads/level1-v0.1.0-rc.4`. No existing ref was updated or deleted.
- The public GitHub Actions REST API identified exactly one matching workflow
  run: run `29688941202`, event `push`, branch
  `level1-v0.1.0-rc.4`, head SHA
  `2546913e38ec273f64417dece2f91df78fd42fc2`, status `completed`, conclusion
  `success`, URL
  `https://github.com/ponchione/revolvr/actions/runs/29688941202`. It started
  at `2026-07-19T13:28:34Z` and completed at `2026-07-19T13:31:54Z`.
- The jobs endpoint returned exactly the ten mandatory distinct jobs. Every
  job reported the exact candidate head SHA and `completed` / `success`:
  - `88198118677` — Go 1.22 source floor and tests —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118677`
  - `88198118646` — Production autonomous strict-fake suite —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118646`
  - `88198118664` — Race tests —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118664`
  - `88198118665` — Vet and module verification —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118665`
  - `88198118653` — Fake-Codex success smoke —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118653`
  - `88198118661` — Fake-Codex verification-failure smoke —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118661`
  - `88198118641` — Build linux/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118641`
  - `88198118668` — Build darwin/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118668`
  - `88198118681` — Build freebsd/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118681`
  - `88198118662` — Build Windows diagnostic stub —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118662`
- Post-CI preservation reverified both RC.4 inventories, all eight sealed
  historical inventories, the exact contents of all three historical
  attestation workflows, all six historical remote refs, exact RC.4 candidate
  readback, RC.4 attestation-ref/tag absence, and all four recorded retired-
  suite absences. RC.1, RC.2, and RC.3 evidence remained immutable.
- Files changed: `.agent/STATE.md`, `.agent/DECISIONS.md`, and
  `.agent/HANDOFF.md` only. No source, dependency, workflow, candidate bundle,
  verification bundle, task entry, or historical evidence changed. No commit
  or push of `main` occurred.
- Verification commands run: candidate `build-instructions.sh --verify`;
  strict SHA-256 checks for both RC.4 inventories and all eight historical
  inventories; exact artifact/source/tree/manifest assertions; raw-Git fetch,
  ancestry, collision, force-with-lease publication, ref readback, and
  preservation checks; finite public REST run polling; exact ten-job REST
  comparison; `git diff --check`; and final worktree inspection.
- Verification result: passed. No `gh`, attestation workflow/ref, suite,
  live/nested Codex or model operation, tag, release, external-use approval,
  or `EXT-20` completion occurred. Blockers for this bounded task: none.
- What remains: in a separate fresh, explicitly authorized pass, add and
  publish a collision-free RC.4 attestation workflow/ref that checks out this
  exact candidate SHA, uses exact Go 1.26.5, reproduces two clean build passes,
  verifies the three sealed artifact hashes and embedded identities, and
  retains the remote artifact/run evidence. No-model suite preparation and
  live quantitative dogfood remain later gates.

## EXT-20 RC.4 Independent Verification And Remote Handoff (2026-07-19)

- Independent controller verification passed both sealed RC.4 inventories,
  exact inventory digests, the candidate self-verifier, all three artifact
  hashes and Go/VCS metadata, source tree and published ancestry, candidate
  ref/attestation-ref/tag collision absence, retained vulnerability and test
  results, eight historical inventories, six historical remote refs, the
  focused Structured Outputs and production happy-path tests, and a fresh
  `go test -count=1 ./...`.
- The local candidate state was committed as
  `1917df5c374f8337a7bebb429478e7e16ea8420d` (`Record reproducible RC.4
  candidate`) and pushed to raw-Git `origin/main`; no candidate ref, workflow,
  CI request, suite, live model operation, tag, release, or external-use
  approval was created by that publication.
- `agent-ext20-rc4-remote.sh` is the next controller-only launcher. Executing
  it supplies explicit authority only to create absent remote ref
  `refs/heads/level1-v0.1.0-rc.4` at exact source
  `2546913e38ec273f64417dece2f91df78fd42fc2` with an empty-expected raw-Git
  force-with-lease and to collect the complete ten-job EXT-15 CI result on that
  SHA. It forbids main publication, attestation, suite preparation, live model
  work, tags, release, and external-use approval.
- What remains: run `./agent-ext20-rc4-remote.sh`, then independently verify
  its ref/CI evidence before committing state. `EXT-20` remains unchecked.

## EXT-20 RC.4 Replacement Candidate — Local Verification (2026-07-19)

- Task selected: the bounded RC.4 local replacement-candidate subtask of
  `EXT-20` only. Candidate `level1-v0.1.0-rc.4` binds release version `0.1.0`
  to exact source commit `2546913e38ec273f64417dece2f91df78fd42fc2`
  and tree `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`. A raw-Git fetch,
  object/tree readback, remote-main readback, and ancestry check proved the
  source is published and reachable from `origin/main` at
  `45a7f2384aaf21e36174660618c5f00a91edb1ab`. The controller helper is absent
  from the source commit and clean source clone.
- Collision checks passed before construction. Local and remote candidate and
  attestation refs, RC.4 tags, candidate/verification bundles, build script,
  local and `origin/main` workflow, public Actions artifact name
  `level1-v0.1.0-rc.4-attestation`, `/tmp` run root, and RC.4 diagnostic names
  were all absent. The retained construction root is
  `/tmp/revolvr-ext20-rc4-build.okV2nU`; it is diagnostic workspace, not
  candidate authority.
- The required first local regressions passed on the exact clean source:
  `TestProductionModelOutputSchemasUseSupportedStructuredOutputsSubset` and
  `TestProductionAutonomousHappyPath`. These results are retained in the
  verification bundle and establish local compatibility/regression proof only;
  they do not claim live API acceptance.
- The immutable candidate bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec/`. Its pinned
  build-instruction SHA-256 is
  `5d87ff8eb5e89865729237dda500c8387ef5880b3c10ea0bd77f896938d606e9`,
  and its 13-entry complete evidence inventory SHA-256 is
  `3535d7a2b46a0dbd3101428b4177e4c46baabc29190e5b1c580d90e6ff033f5d`.
  Two independent `--no-local` clean clones produced byte-identical artifacts:
  Linux amd64
  `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
  Darwin amd64
  `042563f350b71ec8cd5be1b49fc9d948383caa28087c0a5689bd6eb12f3808ab`,
  and FreeBSD amd64
  `128b9f8ced3038a51534da63b9d9ffbaa5ea7341e0ab8dd17102fba86084a8e6`.
- Every artifact independently records Go 1.26.5, command path
  `revolvr/cmd/revolvr`, compiler `gc`, trimpath, exact `GOOS/amd64`, disabled
  CGO, Git source revision, and `vcs.modified=false`. Each has an empty Go
  build ID, one exact 16-byte `main.version` symbol, and exactly one `0.1.0`
  release string; the Linux binary prints exactly `revolvr 0.1.0`.
- The separate immutable verification bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec-verification/`.
  Its 31-entry complete evidence inventory SHA-256 is
  `75a2bcaba12d28d42a5012ad70995f4eb10363e250ec8028350e0802b0b8429c`.
  It retains exact source/tree/tool identities, commands and raw logs, the
  Structured Outputs guard, production happy path, Go-floor/full tests, vet,
  module and vulnerability results, candidate construction and self-check,
  independent build metadata/version assertions, collision checks, artifact
  authority, and before/after historical preservation evidence.
- Required verification passed: Go 1.22.12 source-floor tests; Go 1.26.5 full
  tests; `go vet ./...`; `go mod verify`; `govulncheck ./...` and verbose scan;
  Linux/Darwin/FreeBSD amd64 release builds; two-pass byte comparison; embedded
  metadata/version/build-ID assertions; candidate self-check; verification
  inventory; exact clean-clone authority; and helper exclusion. Govulncheck
  found zero reachable and zero imported-package vulnerabilities. The retained
  module-only finding remains `GO-2026-5024` in the Windows-only
  `golang.org/x/sys/windows` surface at `v0.30.0`, which Revolvr does not call;
  the report names `v0.44.0` as fixed.
- The first independent metadata command stopped on a read-only harness error:
  it indexed `go tool nm -size` symbol type/name as fields 4/5 instead of 3/4.
  The single repair corrected only that assertion and reran the complete check;
  no candidate byte changed. The exact diagnostic is retained as
  `metadata-check-repair.txt` in the verification bundle.
- Preservation passed. Eight available RC.1/RC.2/RC.3 inventories reverified;
  20 historical candidate, verification, failed-diagnostic, workflow, and
  retired-suite targets retained identical content and layout fingerprints;
  all six RC.1/RC.2/RC.3 remote refs retained exact identities. Retired RC.3
  suite `/tmp/revolvr-ext20-rc3.Qghf19/suite` was absent before and after and
  was not recreated or used. No historical suite, operation, run, ref,
  workflow, artifact, hash, bundle, or diagnostic became RC.4 authority.
- Files changed: this state file, `.agent/DECISIONS.md`, `.agent/HANDOFF.md`,
  the new ignored RC.4 pinned build script, candidate bundle, and verification
  bundle. `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked. No Go
  source or dependency changed.
- Verification result: local RC.4 construction passed after the one read-only
  harness repair. No `gh`, commit, push, candidate ref, workflow, remote CI
  request, attestation, suite, live/nested Codex or model operation, tag,
  release, or external-use approval occurred.
- What remains: after independent controller verification and explicit raw-Git
  publication authority, create only collision-free ref
  `refs/heads/level1-v0.1.0-rc.4` at the exact source commit and require every
  EXT-15 CI job to pass on that SHA. Remote artifact attestation and any new
  no-model/live suite remain later separate passes. Blockers for this bounded
  local task: none.

## EXT-20 Structured Outputs Repair Publication And RC.4 Handoff (2026-07-19)

- The operator explicitly authorized committing and pushing the already
  reviewed Structured Outputs follow-up with raw Git. The pre-publication base,
  fetched `origin/main`, and `FETCH_HEAD` all matched
  `45e92302843ad1cafe7a4a6bc58a319d606fb497`; no remote divergence existed.
- Verification passed immediately before publication: formatting of every
  changed Go file; the six-package focused suite; the same focused suite with
  `-race`; the production autonomous happy path; the recursive four-builder
  Structured Outputs guard; `go test -count=1 ./...`; staged and unstaged
  `git diff --check`; and exact staged-scope inspection.
- Raw Git created commit `2546913e38ec273f64417dece2f91df78fd42fc2`
  (`Make model output schemas strict-compatible`) with tree
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`, then pushed `main` from
  `45e9230` to `2546913`. A subsequent fetch confirmed local `HEAD` and
  `origin/main` matched exactly.
- The controller-only `agent-ext20-rc4.sh` binds local RC.4 construction to
  that exact source and tree. It forbids `gh`, commit, push, ref publication,
  remote attestation, suite preparation, live/nested model calls, tags,
  release, or external-use approval. It also forbids reuse or mutation of all
  RC.1, RC.2, and RC.3 authority and evidence.
- What remains: run one fresh bounded pass with `./agent-ext20-rc4.sh` to
  construct and locally verify collision-free RC.4. `EXT-20` remains unchecked;
  no API-acceptance or external-use claim is authorized.

## EXT-20 RC.3 Rejection, Failed First Repair Audit, And Follow-up (2026-07-18)

- Task selected: finish the Structured Outputs compatibility repair exposed by
  the first RC.3 live operation and the failed independent audit of the first
  repair. Work remains based on exact `main` commit
  `45e92302843ad1cafe7a4a6bc58a319d606fb497`; rejected candidate source is
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`.
- Failure authority: suite `/tmp/revolvr-ext20-rc3.Qghf19/suite`, operation
  `ext20-802d9db69596-01`, evidence bundle
  `/tmp/revolvr-ext20-rc3.Qghf19/suite/evidence/repo-a/01-successful-source-change-1`,
  and Codex run `019f761f-078d-7b81-932b-278339f2a000`. The API returned HTTP
  400 `invalid_json_schema`: `In context=('properties', 'conflicts'),
  'uniqueItems' is not permitted.` The terminal outcome is
  `unsafe_or_ambiguous`; statistics and captured state show zero worker
  attempts, zero verification runs, zero audits, zero commits, and no source
  mutation.
- The first repair removed emitted `uniqueItems`, `allOf`, `not`, and `oneOf`,
  moved set uniqueness to semantic Go validation, and passed focused, race,
  strict-fake production, and full-suite tests. Independent audit nevertheless
  rejected it. Authoritative findings were: every object must have
  `additionalProperties: false`; every property must appear exactly once in
  `required`; semantic optionals must be required-nullable or required-empty;
  unconstrained objects are prohibited; and the finite keyword denylist was
  not a supported-subset compatibility guard.
- The follow-up audits and repairs exactly the four ordinary production
  builders: supervisor `DecisionOutputSchema`, planner `PlanningOutputSchema`,
  auditor `AuditOutputSchema`, and corrector `CorrectionOutputSchema`. Every
  object and definition is concrete and closed, every declared property is
  required exactly once, every array has concrete `items`, and supported null
  or empty-array wire values preserve existing Go zero/nil meaning.
- Supervisor root optionals, `strategy`, `needs_input_question`, and
  `child_task` optional collections now use required null/empty forms. A
  nullable worker-profile enum includes JSON null. Required-null
  `content_sha256` still decodes to the Go zero value and `ParseDecision`
  computes and validates its deterministic content identity.
- Planning `plan_step.evidence`, `plan_step.rationale`,
  `task_plan.supersedes_plan_id`, `acceptance_criterion.evidence`, and
  `acceptance_criterion.rationale` preserve absence as empty slices, empty Go
  strings decoded from null, or the existing zero predecessor identity. Go
  status/revision validation still rejects fabricated rationale or authority.
- Audit `report.findings`, `verification_summary.command`, both tiered fields,
  `mutation.decision_id`, and `provenance.latest_source_mutation` now have exact
  required empty/nullable representations. The former unconstrained
  `verification_summary.tiered` is a closed model projection fixed to null;
  the host compares that projection and deterministically reattaches the exact
  trusted full tiered result before canonicalization and persistence. The
  optional final-gate projection has exact closed plan/tier definitions.
- Correction now requires every root property, models
  `VerificationFailureTarget` exactly, models every `EvidenceReference` item as
  a concrete closed object, and uses null versus empty `finding_ids` for the
  exclusive authority partition. Strict decoding and Go partition, outcome,
  evidence, duplicate, and authority validation remain unchanged.
- Semantic validation remains fail-closed without deduplication or reordering:
  supervisor and correction finding IDs, resolved/remaining correction IDs,
  child `depends_on`/`tags`/`conflicts`, needs-input option IDs and meanings,
  independent-work IDs and exact option identities, audit finding IDs, and
  audit verification-tier identity arrays all reject duplicates. Focused
  decoded-response and validator tests prove null/empty decoding and continued
  conditional validation.
- The stdlib-only recursive regression validator enumerates all four
  production builders and uses an explicit current-documentation allowlist. It
  distinguishes schema keywords from property and `$defs` names, checks every
  object including nullable variants, requires array `items`, resolves local
  `$ref` targets, recursively validates `$defs` and `anyOf`, and reports exact
  JSON paths. Negative fixtures cover `contains`, every named unsupported
  composition/uniqueness keyword, missing/true `additionalProperties`, missing,
  duplicate, and unknown `required` entries, unconstrained objects, missing
  array `items`, nullable enums without null, bad definitions, and unresolved
  refs.
- Before editing, both RC.3 evidence manifests passed with `sha256sum -c
  --strict`. Read-only regular-file content/layout fingerprints captured with
  `GIT_OPTIONAL_LOCKS=0` were evidence bundle
  `e47642eb4e8ade29ff213a3012891dc11a4bf800b654f80cb8c08a527564c689` /
  `bded4ce56ff6b2d8407978a40a3945b06eb1c0e982ec942e3671b14258b1b335`
  and whole suite
  `e070947f3a6cc3d0f598a3a78948757d7c1c0837c8baad70028cffb54b5734be` /
  `84e24f06525d81af2ff84061488d19170cc9791ca3139632892e3c5bf0431d58`.
  After all code checks, both manifests passed again and all four fingerprints
  matched these pre-edit values exactly.
- Files changed: `internal/supervisor/schema.go`,
  `internal/supervisor/prompt_schema_test.go`,
  `internal/autonomousplanning/schema.go`,
  `internal/autonomousplanning/contracts_test.go`,
  `internal/autonomous/correction.go`,
  `internal/autonomous/correction_test.go`,
  `internal/autonomous/contracts_test.go`,
  `internal/autonomous/needs_input_test.go`,
  `internal/autonomousaudit/schema.go`,
  `internal/autonomousaudit/contracts.go`,
  `internal/autonomousaudit/contracts_test.go`,
  `internal/autonomousauditapply/apply.go`,
  `internal/autonomousauditapply/apply_test.go`,
  `internal/autonomouscycle/worker_prompt.go`,
  `internal/autonomouscycle/schema_compatibility_test.go`, this file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. No dependency was added.
- Verification passed: `gofmt -w` on every changed Go file; focused ordinary
  and `-race -count=1` tests for `internal/autonomous`, `internal/supervisor`,
  `internal/autonomousplanning`, `internal/autonomousaudit`,
  `internal/autonomouscycle`, and changed `internal/autonomousauditapply`;
  `go test -count=1 ./internal/app -run
  '^TestProductionAutonomousHappyPath$'`; final `go test -count=1 ./...`; final
  recursive four-builder compatibility test; and `git diff --check`. Direct
  generated-schema inspection found zero structural violations and only
  allowlisted keywords; generated SHA-256 values were supervisor
  `5ef89243892e156bfdf098c132ea42ddc0a0ff74bd12af5276a493ac16be6c76`,
  planning `b4d088d7833a604ae4e91999dbc52a49b925d75ecfb3953dc56bdcceca2a1e09`,
  audit `899b551851549974b07c5352ba675e11f598763433c31c458a4c21e1e37e5eb3`,
  and correction
  `0badb90760b7ca013c2cc7cb0ebf4c54f404a9d749dd9f5d29e4051e5812f022`.
- RC.3 is permanently retired and must never be retried, reconciled,
  relabeled, or reused. RC.1 and RC.2 remain equally immutable. No `gh`,
  commit, push, tag, branch/ref publication, release, live/nested model
  operation, or external-use approval occurred. No API-acceptance claim is
  made. `EXT-20` remains unchecked. RC.4 remains blocked until this follow-up
  passes a new independent audit and is separately committed with explicit
  authority; this pass did not construct it.

## EXT-20 RC.3 Guarded No-Model Suite Preparation (2026-07-18)

- Task selected: the bounded RC.3 no-model suite-preparation subtask of
  `EXT-20`. The guarded suite now binds exact source
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, Linux SHA-256
  `9e9c13f43977edf49e7e6385c595aa20a01c16308ddfea1c30455ea88252ae9b`,
  and immutable bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.3-a16ea1bdc1a4`. Release
  output remains `revolvr 0.1.0`; Codex authority remains exact package
  `@openai/codex@0.144.4`, output `codex-cli 0.144.4`, and SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  No plan, schema, scenario, threshold, configuration, confirmation value,
  Go source, or dependency changed.
- Files changed: `scripts/dogfood-external-level1-suite.sh`, this state file,
  and `.agent/HANDOFF.md`. `.agent/TASKS.md` remains unchanged with `EXT-20`
  unchecked. `.agent/DECISIONS.md` remains unchanged because this pass applies
  the already settled immutable RC.3 authority.
- No-model verification passed: `bash -n` for suite and collector; the complete
  RC.3 candidate bundle verifier; suite `--static`; two collector
  `--fixture-only` collections; both `--verify-manifest` checks; canonical
  manifest comparison after excluding only `collected_at_utc`; `go test
  -count=1 ./...`; and `git diff --check`. The retained fixture root is
  `/tmp/revolvr-ext20-rc3-collector.Uad2DH`; both raw manifest hashes are
  `5169090fe855302da1fe70ca98535d7b66e4769c1af81ddec27affd9e9fc64e9`
  and the timestamp-excluded canonical manifest hash is
  `01a47fd5ab7f7eb8e7def144ce7cbef17d5b306366cb2967896fb7b874e8452c`.
- The retained prepared suite authority is
  `/tmp/revolvr-ext20-rc3.Qghf19/suite`. Preparation used a new parent from
  exact template `/tmp/revolvr-ext20-rc3.XXXXXX` and only `--prepare
  --run-root <parent>/suite --install-codex`; it installed the pinned package
  and started no model. `authority.tsv` SHA-256 is
  `adc8095701e1fa6fdcd6180df93ea88ccac6f1d4d21cba1a751476a7a1ef3fb4`;
  `operation-plan.tsv` SHA-256 is
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`.
- Independent read-only inspection verified the authority hash; exact
  candidate hash/version/source metadata and `vcs.modified=false`; exact
  Codex package/version/hash; effective configuration schema v8; source-writer
  lock `timeout=32m0s heartbeat_interval=10m40s required=32m0s`; the exact
  11-row plan and ten unique ready tasks; tracked disposable markers; and both
  clean `main` repositories. `repo-a` is at
  `07239693a95a1c73b61de77b74ade0a234e84075`; `repo-b` is at
  `2de82370e6c8bd3775e25da783fc7d6dcf0e0b5c`. There are zero runtime
  operation manifests, zero collector manifests, and zero aggregate entries.
- Inspection set `GIT_OPTIONAL_LOCKS=0` so read-only Git commands could not
  persist optional index refreshes. Complete regular-file content fingerprint
  `90424fc5544fd3be146eef965a95b5300a910036e0a19703672fc95f1fb756ae`
  and layout/type/mode/owner/size/link/inode/mtime/ctime fingerprint
  `8fdd4272d36eefe1f9c99c4793bc47d51db5751ed12450b986755a875ae467b7`
  were identical before and after every authority, config, doctor, Git,
  manifest, and aggregate check. The exact empty-confirmation guard was tested
  in isolation and refused before any prepared-root read or collector call;
  neither `--live` nor the confirmation value was passed to the suite.
- A first preparation at `/tmp/revolvr-ext20-rc3.5TQPha/suite` is retained
  only as a failed inspection diagnostic. An independent doctor/status sweep
  was run without disabling Git optional locks, and the whole-root comparison
  caught an optional stat-cache rewrite of `repo-b/.git/index`. Its source
  worktree stayed clean, both HEADs stayed fixed, and it has zero operations,
  collector manifests, and aggregate entries, but it is not live authority and
  must not be used. The single repair attempt produced the verified root above.
- Verification result: passed for the tracked three-constant RC.3 update and
  the repaired no-model prepared suite. RC.1 and RC.2 remain rejected history;
  no historical ref, bundle, workflow, artifact record, hash, diagnostic, or
  available root was reused or changed. No `gh`, commit, push, tag, live or
  nested Codex/model operation, or external-use approval occurred.
- Independent controller verification repeated shell syntax, the complete
  candidate-bundle verifier, suite `--static`, exact candidate and Codex
  identities, prepared-authority verification, the 11-row/ten-task plan, both
  fixed clean repository heads, effective schema v8, and all ten task-specific
  doctor checks with the exact 32-minute lock authority. It confirmed zero
  runtime operations, collector manifests, and aggregate files. Controller
  content fingerprint
  `ba6436c19168f9af8b615f16b655b6f12e8fd86c6829bafbab45105230af6e9c`
  and layout fingerprint
  `154879f66815174b9bb4f5e3ae30380327b92a6da7d811b5eca75780d191f403`
  were unchanged before/after inspection with `GIT_OPTIONAL_LOCKS=0`.
  `go test -count=1 ./...` and final diff hygiene passed. No live argument,
  confirmation value, model operation, or prepared-root mutation occurred.
- Remaining separately confirmed live command, recorded but not executed:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-rc3.Qghf19/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. After that pass, every EXT-17 manifest
  and the EXT-20 aggregate still require independent validation. `EXT-20`
  remains unchecked until all quantitative and zero-tolerance thresholds pass.
  The intentional blocker is the separate live confirmation, which was not
  supplied in this no-model pass.

## EXT-20 RC.3 Remote Artifact Attestation Result (2026-07-18)

- The controller independently parsed the new workflow, asserted its exact
  trigger, permissions, candidate checkout, toolchain, cache policy, constants,
  upload contract, and shell syntax, then executed the complete unmodified
  embedded shell under host Go 1.26.5 from a fresh detached candidate clone.
  Both independent no-local build passes reproduced all three exact artifact
  hashes and passed byte, metadata, version, authority-manifest, and retained-
  output checks. The controller root is
  `/tmp/revolvr-controller-rc3-attestation.wzxrrfaj`; it contains 21 retained
  regular single-link files. Historical RC.1/RC.2 workflow hashes, the five
  pre-existing remote release refs, exact RC.3 source commit/tree, and diff
  hygiene also passed independent readback before commit.
- Raw Git committed the reviewed workflow and state as
  `80441464d55af466bbea15f20448099e2a163684` (`Add RC.3 artifact attestation
  workflow`), pushed `main`, and published the previously absent ref
  `level1-v0.1.0-rc.3-attestation` at that exact workflow commit. Remote
  readback preserved candidate ref `level1-v0.1.0-rc.3` at exact source
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`. No historical ref moved.
- Dedicated GitHub Actions run `29651665454` completed `success` at the exact
  workflow commit. Job `88098916882`, `Rebuild and attest Level 1 RC.3
  candidate`, passed checkout, pinned Go setup, both clean-clone rebuilds and
  attestations, retained-authority upload, and all post steps. Run evidence:
  `https://github.com/ponchione/revolvr/actions/runs/29651665454`.
- The retained unexpired artifact is
  `level1-v0.1.0-rc.3-attestation`, artifact ID `8431664217`, size 70,202,355
  bytes, GitHub artifact digest
  `sha256:8ac9f82795233b4808fc8c2fc895a11d1fb622e30272f73f90b1f68218c99cd1`,
  creation `2026-07-18T16:20:16Z`, and expiry
  `2026-10-16T16:17:29Z`. Public REST metadata was inspected; the archive
  endpoint requires authentication and returned HTTP 401, so no independent
  claim of downloading the remote ZIP is made.
- General push-triggered CI run `29651665483` also completed `success` at the
  exact workflow commit. All ten required jobs passed: Go 1.22 source floor
  and tests; Darwin, Linux, and FreeBSD amd64 builds; Windows diagnostic stub;
  vet and module verification; both fake-Codex smokes; race tests; and the
  production autonomous strict-fake suite.
- RC.3 remote candidate and artifact prerequisites are complete. `EXT-20`
  remains unchecked and external use remains unapproved. The next bounded pass
  is to update the guarded Level-1 suite from rejected RC.2 to immutable RC.3,
  prepare a fresh collision-free no-model root, and stop before any separately
  confirmed live real-Codex invocation. No `gh`, tag, live/nested Codex, or
  model operation was used.

## EXT-20 RC.3 Remote Artifact Attestation Workflow (2026-07-18)

- Task selected: the bounded RC.3 remote-artifact prerequisite of `EXT-20`
  only. New separate workflow
  `.github/workflows/level1-rc3-candidate-attestation.yml` triggers only on a
  push to `level1-v0.1.0-rc.3-attestation` and checks out exact candidate
  source `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, not the trigger HEAD.
- The workflow installs exact Go 1.26.5 with cache restoration disabled and
  makes two independent clean `--no-local` clones. Each pass has its own Go
  build and module caches and builds Linux, Darwin, and FreeBSD amd64 with
  disabled CGO, local toolchain selection, module-readonly mode, trimpath,
  explicit clean VCS metadata, an empty build ID, and
  `main.version=0.1.0`.
- Every pass artifact must expose exact Go version, command path, compiler,
  trimpath, target, CGO, Git source revision, and `vcs.modified=false`
  metadata plus one exact 16-byte `main.version` symbol and one exact `0.1.0`
  string. Both Linux copies must report `revolvr 0.1.0`. The workflow requires
  byte-identical pass pairs and exact SHA-256 values Linux
  `9e9c13f43977edf49e7e6385c595aa20a01c16308ddfea1c30455ea88252ae9b`,
  Darwin
  `ee6db29cfcbcbd2e645184927fc5d7348ed924d6036a46d7b23eae55b5b43fd4`,
  and FreeBSD
  `6a42dc423ab1975e8ea4296f56be6c9a3773d9ccfda1c57244a50261e64f368a`.
  One uploaded artifact retains both binary sets, all six build-metadata and
  version assertions, all six hashes, the three-pair reproducibility table,
  and the exact tabular authority manifest.
- Files changed: the new RC.3 workflow and this state file only.
  `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked, and
  `.agent/DECISIONS.md` remains unchanged because the workflow implements the
  already settled RC.3 authority. No Go source or dependency changed.
- Verification commands run: all required durable-state and newest RC.3
  local/remote authority reads; raw-Git exact candidate object/tree and remote
  ref/collision checks; PyYAML BaseLoader parsing with exact trigger, checkout,
  toolchain, cache, clone, target, build, metadata, version, hash, and upload
  assertions; `bash -n` on the extracted embedded shell; and execution of the
  complete unmodified embedded shell under host Go 1.26.5 from a fresh
  detached exact-source clone. The shell created two further independent clean
  clones and passed all six builds and hashes. An independent retained-output
  inspection verified 21 regular single-link files, six hash-manifest rows,
  three byte-identical pairs, three exact authority rows, two clean source
  passes with separate caches, and the complete authority/reproducibility
  manifests. The retained local verification root is
  `/tmp/revolvr-ext20-rc3-attestation.desFNJ`; its relative-file/hash stream
  digest is
  `3303495dac203c5206c36d477f173c11c0ccc57e850ac92ab11e46aa6cd3daaa`.
  Workflow-only whitespace checks and final `git diff --check` passed. No Go
  test was required because no Go file changed.
- Preservation evidence: the RC.1 and RC.2 workflows remain exact SHA-256
  `d1314182a0cffd78927e6a5cc688e370c42f3d17a4e4ffe426f647a384c40a41`
  and
  `4c96ec62a3757878926b62aee65ce8ba3ec6ac2148ac251622ab294109312c6d`.
  Every available RC.1/RC.2/RC.3 sealed candidate and verification inventory,
  plus both inventoried RC.1/RC.2 failed candidate diagnostics, reverified.
  The two RC.3 fail-closed verification diagnostics were not used or written.
  Raw Git still resolves RC.1, RC.1 attestation, RC.2, RC.2 attestation, and
  RC.3 candidate refs to `ed65049fba6bf82852fd406ebc17afa90a953e3f`,
  `a1afdd73a7bfb03e9e5ef361616604115f9db5b8`,
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`,
  `7038030d07c9eb1b76e0af2a3fdc84154d9b6fe2`, and
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, respectively. No remote
  artifact or ref was mutated. The three historical temporary suite roots
  `/tmp/revolvr-ext20-fresh.OmCBwv/suite`,
  `/tmp/revolvr-ext20-live2.3ZVQcm/suite`, and
  `/tmp/revolvr-ext20-rc2.96ibla/suite` were already absent during the initial
  pre-change inspection and remained absent; this pass did not remove,
  recreate, or represent them as locally available. Their prior recorded
  fingerprints remain unchanged in the sealed RC.3 verification bundle.
- Verification result: passed for the workflow structure, exact constants,
  embedded shell syntax, complete two-pass execution, hashes, byte equality,
  metadata, version authority, retained output, available sealed inventories,
  raw refs, and diff hygiene. No remote workflow, live/nested Codex, model,
  commit, push, tag, candidate-source/ref mutation, or external-use approval
  was started.
- Collision-free raw-Git attestation ref to publish after controller review
  and commit: `refs/heads/level1-v0.1.0-rc.3-attestation`. Raw Git found it
  absent at `origin`; publication must use the reviewed workflow commit as its
  tip and must not move exact candidate ref `level1-v0.1.0-rc.3`.
- What remains: the controller must independently verify and commit this local
  change, publish only the collision-free attestation ref, require the remote
  job to succeed, and record its exact workflow commit, run/job URL and
  conclusions, uploaded artifact ID/name/size/digest/expiry, and comparison
  with the local RC.3 authority. Only after that remote evidence exists may a
  fresh collision-free no-model RC.3 dogfood suite be prepared. `EXT-20`
  remains unchecked; external use remains unapproved.

## EXT-20 RC.3 Exact-Candidate Remote CI (2026-07-18)

- Independent controller verification passed both sealed RC.3 bundles, exact
  inventory digests, source/tree authority, a third non-local clean-clone
  rebuild, byte equality for all three supported artifacts, and a fresh
  no-model RC.3 binary config/doctor smoke.
- The local evidence/helper update was committed as `41d5319`. Raw Git pushed
  `main` and published collision-free candidate branch
  `level1-v0.1.0-rc.3` at exact source commit
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`; remote readback resolves that
  ref to the same SHA. RC.1 and RC.2 refs were not moved.
- Push-triggered GitHub Actions CI run `29642126354` completed `success` on
  that exact candidate SHA. All ten required jobs passed: Go 1.22 source floor
  and tests; Darwin, Linux, and FreeBSD amd64 builds; Windows diagnostic stub;
  vet and module verification; fake-Codex success and verification-failure
  smokes; race tests; and the production autonomous strict-fake suite. Run
  evidence: `https://github.com/ponchione/revolvr/actions/runs/29642126354`.
- No `gh`, tag, attestation workflow, live/nested Codex, model operation, or
  external-use approval was used or created. EXT-20 remains unchecked.
- The next bounded pass is a separate collision-free RC.3 exact-checkout
  Go 1.26.5 artifact-attestation workflow. It must reproduce both clean build
  passes, all three recorded RC.3 hashes and embedded identities, retain the
  remote authority artifact, and preserve RC.1/RC.2 workflows and evidence.

## EXT-20 RC.3 Replacement Candidate — Local Verification (2026-07-18)

- Task selected: the bounded RC.3 local replacement-candidate subtask of
  `EXT-20` only. Candidate `level1-v0.1.0-rc.3` binds release version `0.1.0`
  to exact clean source commit
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c` and tree
  `23c0d27fc62be5f41feb45192e74f1df8ecff3fa`. Local and remote candidate/
  attestation refs, RC.3 bundle paths, and `/tmp` RC.3 construction roots were
  absent before construction. The controller-created untracked
  `agent-ext20-rc3.sh` was absent from the source commit and every clean clone.
- The focused source-lock proof ran before candidate construction against the
  exact clean source. Ordinary and race tests across `internal/lock`,
  `internal/runonce`, `internal/app`, and `internal/cli` passed the required
  window calculation, safe defaulting, invalid/overflow refusal, effective
  fingerprint, config rendering, ready doctor, and zero-model preflight
  regressions. Exact retained outputs are `source-lock-test.txt` and
  `source-lock-race-test.txt` in the verification bundle.
- The immutable candidate bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.3-a16ea1bdc1a4/`. Its pinned
  build-instruction SHA-256 is
  `2deaa06d380dfd7d86277e2090229ba1d212e1bad85c6b3db6a83a31036c1405`;
  its 13-entry complete evidence inventory SHA-256 is
  `766856c2783073c8ffa10cbce0e3c0a9f8ebee4db1785c309f6c5680f5e5ddae`.
  Two independent `--no-local` clean clones produced byte-identical artifacts:
  Linux amd64
  `9e9c13f43977edf49e7e6385c595aa20a01c16308ddfea1c30455ea88252ae9b`,
  Darwin amd64
  `ee6db29cfcbcbd2e645184927fc5d7348ed924d6036a46d7b23eae55b5b43fd4`,
  and FreeBSD amd64
  `6a42dc423ab1975e8ea4296f56be6c9a3773d9ccfda1c57244a50261e64f368a`.
- Every artifact independently records Go 1.26.5, compiler `gc`, trimpath,
  its exact `GOOS/amd64`, `CGO_ENABLED=0`, Git VCS authority, source revision
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, and
  `vcs.modified=false`. Each contains the exact 16-byte `main.version` symbol
  and exactly one `0.1.0` release string; the Linux artifact prints exactly
  `revolvr 0.1.0`.
- The separate immutable verification bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.3-a16ea1bdc1a4-verification/`.
  Its 29-entry complete evidence inventory SHA-256 is
  `006cb0b7f2215878e757ae8ee104bb1b88d5ae31b8661168bcf5c705d08353ff`.
  It retains exact source/tree/tool identities, commands and raw logs, focused
  source-lock proof, candidate manifest and inventory, two-build
  reproducibility, independent build metadata/version assertions, collision
  checks, all four preserved remote refs, and before/after preservation
  inventories.
- Required verification passed: Go 1.22.12 source-floor tests; Go 1.26.5 full
  tests; `go vet ./...`; `go mod verify`; `govulncheck ./...` and verbose scan;
  Linux/Darwin/FreeBSD amd64 release builds; exact two-pass artifact comparison;
  embedded version/source/tool/target/CGO/VCS metadata; candidate self-check;
  verification-bundle inventory check; clean-clone authority; and exclusion of
  the controller helper. Govulncheck found zero reachable and zero imported-
  package vulnerabilities. The retained module-only finding remains
  `GO-2026-5024` in the Windows-only `golang.org/x/sys/windows` surface at
  `v0.30.0`, which Revolvr does not call; the report names `v0.44.0` as fixed.
- Two fail-closed verification-assembly diagnostics are retained unchanged at
  suffixes `.failed-readonly-commands-append` and
  `.failed-sealed-before-inventory`. The first refused an append after an
  early read-only install; the second refused inventory creation after an
  early directory seal. Their relative content/layout fingerprints are,
  respectively,
  `5b0a359ff8432730dad07e25b290bc6b3c52b7811d9b9e2c8f2324e4804bd8e7` /
  `6264d0f32ac7f08d413d245cf85a122b226f9fa33811348efe9d14f7605aee65`
  and
  `006cb0b7f2215878e757ae8ee104bb1b88d5ae31b8661168bcf5c705d08353ff` /
  `69d555e7193ea069b7a2be176d1f3455434bd4b3a748a9580fee08d333c8fba9`.
  The authoritative finalization created and verified both inventories before
  sealing the completed bundle read-only.
- Preservation passed. RC.1 and RC.2 candidate/verification/failed bundles,
  both existing attestation workflows, both earlier retired roots, and failed
  RC.2 suite `/tmp/revolvr-ext20-rc2.96ibla/suite` retained identical content
  and layout/metadata fingerprints. Raw Git readback preserved candidate refs
  RC.1 `ed65049fba6bf82852fd406ebc17afa90a953e3f`, RC.2
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`, and attestation refs RC.1
  `a1afdd73a7bfb03e9e5ef361616604115f9db5b8` and RC.2
  `7038030d07c9eb1b76e0af2a3fdc84154d9b6fe2`. No old artifact, hash,
  workflow, ref, run root, or evidence file changed.
- Files changed: this state file, `.agent/DECISIONS.md`, the new ignored RC.3
  pinned build script, candidate bundle, verification bundle, two retained
  assembly diagnostics, and controller-created fresh-session helper
  `agent-ext20-rc3.sh`. `.agent/TASKS.md` remains unchanged with `EXT-20`
  unchecked. No Go source, dependency, candidate ref, workflow, dogfood suite,
  tag, live/nested Codex, model operation, remote CI request, or external-use
  approval was created during local candidate construction.
- Independent controller verification passed both sealed-bundle self-checks,
  exact inventory digests, candidate source/tree authority, retained Go-floor/
  release/focused-race/vulnerability evidence, and collision-free RC.3 refs.
  A third independent non-local clean clone rebuilt both passes and reproduced
  all three artifact bytes exactly. The rebuilt Linux binary then passed a
  fresh disposable-repository no-model smoke: config check reported effective
  schema v8 and `32m0s`/`10m40s` lock authority, doctor reported the same
  required window and `Ready: true`, Git stayed clean, and no task operation
  was created.
- Exact candidate-ref publication and mandatory CI completion are recorded in
  `EXT-20 RC.3 Exact-Candidate Remote CI` above. The next separate pass must
  add and run a collision-free exact-checkout Go 1.26.5 RC.3 artifact
  attestation before preparing a new no-model dogfood suite. Blockers for that
  bounded workflow task: none.

## EXT-20 RC.2 Source-Lock Configuration Defect (2026-07-18)

- Task selected: the bounded `EXT-20` configuration-normalization/preflight
  repair only. The retained terminal evidence for operation
  `ext20-3601e63c616b-01` was read before source changes. Its `config check`
  recorded Codex timeout `30m0s`, Git timeout `30s`, and effective-config
  schema v7; doctor reported `Ready: true`; execution then terminalized
  `unsafe_or_ambiguous` because the effective source-writer timeout was `5m0s`
  while autonomous execution required `32m0s`.
- The retained operation proves zero model attempts: task-operation statistics
  contain one started/completed cycle but zero attempts, verification runs,
  audits, corrections, commits, and checkpoints. No Codex `exec` evidence,
  receipt, or model run exists. The failure occurred at autonomous-cycle
  configuration validation before supervisor/model invocation.
- Root cause: `runonce` finalized Codex and Git timeouts but independently used
  a fixed five-minute source-writer default. Supervisor and autonomous-cycle
  validation separately required `CodexTimeout + 2*GitTimeout + 1m`, allowing
  config check/doctor to admit authority that execution rejected.
- Fix: `internal/lock` now owns one overflow-safe required-window calculation
  shared by effective normalization, supervisor, and autonomous cycle.
  Unspecified effective lock authority is derived only after final Codex/Git
  timeouts; valid explicit longer authority is preserved; negative, too-short,
  and overflowing authority fails closed; heartbeat authority is derived only
  after the final lock timeout. Config check and doctor now render the exact
  effective timeout, heartbeat, and required window. The changed effective
  contract is fingerprinted as `revolvr-effective-run-config-v8`.
- Files changed: `internal/lock/source_writer_window.go` and its test;
  `internal/runonce/runonce.go`, `effectiveconfig.go`, and focused tests;
  `internal/supervisor/execution.go`; `internal/autonomouscycle/cycle.go`;
  `internal/app/external_admission.go`, `external_preflight_test.go`,
  `config_numeric_test.go`, and `app_test.go`; `internal/cli/config.go`,
  `root_test.go`, and `doctor_test.go`; the EXT-17 fixture's effective-schema
  projection in `scripts/dogfood-external-level1.sh`; the controller-created
  fresh-session helper `agent-ext20-source-lock.sh`; this state file; and
  `.agent/DECISIONS.md`.
- Verification commands run: `gofmt` on every changed Go file; focused
  ordinary tests across `internal/lock`, `internal/runonce`, `internal/app`,
  and `internal/cli`; the same source-window/config/preflight/CLI regressions
  with `-race`; ordinary supervisor/autonomous-cycle and package tests;
  `bash -n scripts/dogfood-external-level1.sh`; `go test -count=1 ./...` after
  the final repair; CLI `config check` and `doctor --for attended-task`
  no-model probes; tracked/untracked Go formatting inspection; source-schema
  scans; raw-Git candidate/attestation ref inspection; protected evidence and
  bundle content/layout fingerprints; and `git diff --check`.
- Verification result: passed. The focused 30m Codex/30s Git regression
  derives timeout `32m0s` and heartbeat `10m40s`; a ready attended doctor
  exposes the same required authority with zero model invocations. Custom
  45s/12s authority derives `2m9s`, explicit valid 5m/30s authority is
  retained, and short, negative, or overflowing authority is refused. The
  standalone CLI probe printed schema v8 and the same 32m/10m40s authority;
  its doctor source-lock check was OK (the disposable probe as a whole was not
  ready because initialization refused its umask-created `.git` mode 0775).
- Independent controller verification reviewed every production/test diff,
  repeated changed-Go formatting and shell syntax checks, ran the focused
  source-window/config/preflight tests ordinarily and with the race detector,
  ran supervisor and autonomous-cycle package tests, collected and verified a
  fresh EXT-17 fixture manifest, ran focused CLI race tests, and reran
  `go test -count=1 ./...` plus `git diff --check`; all passed.
- Preservation: RC.1, RC.2, all candidate/attestation refs and bundles, both
  earlier retired roots, and failed RC.2 suite root
  `/tmp/revolvr-ext20-rc2.96ibla/suite` were not edited. Repeated content and
  layout/metadata fingerprints agreed at close. No live/nested Codex or model
  operation, external-repository edit, commit, push, tag, ref update, candidate
  build, or publication was performed.
- Release consequence: RC.2 remains immutable rejected historical evidence for
  the quantitative gate; this material configuration defect invalidates its
  affected live operation and prevents RC.2 from satisfying `EXT-20`. No RC.3
  was built or published in this pass. `EXT-20` remains unchecked.
- What remains: after independent controller verification/commit, build a new
  collision-free replacement candidate (RC.3), reproduce and attest its exact
  supported-platform artifacts and remote CI, prepare a fresh collision-free
  no-model suite, then separately confirm and run all quantitative real-Codex
  scenarios and independently verify every manifest/aggregate. Blockers for
  this bounded code repair: none.

## EXT-20 RC.2 Guarded No-Model Suite Preparation (2026-07-18)

- Task selected: the bounded RC.2 no-model preparation subtask of `EXT-20`.
  The guarded external Level-1 suite now binds exact candidate source
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`, exact Linux SHA-256
  `06c1258a947def8c53e03bfd79944bb002351358fc8dfecd35682ab7532b5010`,
  and immutable bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.2-eeaaf50b52fd`. Release output
  remains exactly `revolvr 0.1.0`; Codex authority remains exactly
  `@openai/codex@0.144.4`, `codex-cli 0.144.4`, and SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  No plan, schema, scenario, threshold, configuration, or confirmation value
  changed.
- Files changed: `scripts/dogfood-external-level1-suite.sh`, the controller-
  created fresh-session helper `agent-ext20-rc2-suite.sh`, and this state file.
  `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked, and
  `.agent/DECISIONS.md` remains unchanged because this applies already settled
  RC.2 authority.
- Verification commands run: all required durable-state and newest RC.2
  local/remote authority reads; raw-Git readback of all four RC.1/RC.2 refs;
  before/after content and layout fingerprints for both attestation workflows,
  the RC.1/RC.2 candidate and verification bundles, the retained RC.2 failed
  diagnostic, both retired RC.1 live roots, and the pre-existing helper;
  `bash -n` for the suite and collector; the complete RC.2 candidate bundle
  `--verify`; suite `--static`; two collector `--fixture-only` runs, both
  `--verify-manifest` checks, and their canonical-manifest comparison after
  removing only `collected_at_utc`; `git diff --check`; and
  `go test -count=1 ./...`. All passed.
- A new parent was created with exact template
  `/tmp/revolvr-ext20-rc2.XXXXXX`, and no-model preparation with
  `--prepare --run-root <parent>/suite --install-codex` passed. The retained
  prepared suite is `/tmp/revolvr-ext20-rc2.96ibla/suite`. Its authority hash
  verifies; candidate version/hash/source and isolated Codex package/version/
  hash agree exactly; the 11-row plan names exactly 10 ready tasks; repository
  `repo-a` is clean at `ea72ad021d711b21522e4cd586400ae625ce308f`
  and `repo-b` is clean at `99e41baade47d24abc17e6ff0d6b18acd9765bbb`;
  both retain the exact tracked disposable marker. There are zero collector
  manifests, zero runtime operation manifests, and zero aggregate entries.
- Independent inspection left the prepared suite byte-for-byte and
  metadata-for-metadata unchanged, with regular-file fingerprint
  `37546e2d33e0239e63a029e8ca303a52a3c22d4c10abd939d50c6f72551f32b0`
  and layout/metadata fingerprint
  `ee0babb21f61731de593870b20e7c5693215608ffc6007faa86245c1cf56cb5d`.
  The exact empty-confirmation guard was executed in isolation and refused
  before the first prepared-root read or collector call. In accordance with
  the operator direction, no `--live` argument or confirmation value was
  passed to the suite, and no live, nested Codex, or model operation started.
- Verification result: the tracked shell-only RC.2 update and the new no-model
  suite preparation passed. RC.1, both retired live roots, every RC.1/RC.2
  ref, bundle, workflow, artifact, hash, and retained diagnostic remained
  unchanged. No commit, push, tag, or external-use approval was created.
- Independent controller verification repeated helper/suite/collector syntax,
  suite static verification, RC.2 bundle verification, exact prepared
  authority/candidate/Codex checks, all ten task-ready doctor checks, both Git
  cleanliness checks, and the zero-manifest/zero-aggregate assertions. A full
  prepared-file content inventory was identical before and after inspection.
- What remains: a separately confirmed live pass must execute and then
  independently verify all retained manifests and aggregate thresholds with
  the exact command below. It was recorded but not executed:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-rc2.96ibla/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. `EXT-20` remains unchecked until that
  live suite and `--verify-suite` complete successfully.
- Blocker for the remaining EXT-20 work: the separate live confirmation was
  intentionally not supplied in this no-model pass.

## EXT-20 RC.2 Remote Artifact Attestation Result (2026-07-17)

- Raw Git published collision-free attestation ref
  `level1-v0.1.0-rc.2-attestation` at exact workflow commit
  `7038030d07c9eb1b76e0af2a3fdc84154d9b6fe2`; remote readback preserved the
  candidate ref at exact source commit
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`.
- Dedicated GitHub Actions run `29621464972` completed `success` at the exact
  workflow commit. Job `88017202674`, `Rebuild and attest Level 1 RC.2
  candidate`, passed its exact-candidate checkout, pinned Go setup, two-clean-
  clone rebuild and attestation, and retained-authority upload steps. The
  workflow therefore enforced all six recorded hashes, all three byte-for-byte
  build-pair comparisons, embedded build/version metadata, and the authority
  manifest before upload. Run evidence:
  `https://github.com/ponchione/revolvr/actions/runs/29621464972`.
- The retained unexpired artifact is
  `level1-v0.1.0-rc.2-attestation`, artifact ID `8422371223`, size
  70,182,577 bytes, with GitHub artifact digest
  `sha256:f68f6b5a02ce5f18c31ba50b11dc4a0e653145161b0636dce265401216902018`
  and expiry `2026-10-15T23:44:45Z`. The public REST metadata was inspected;
  its archive endpoint requires authentication and returned HTTP 401, so no
  independent claim of downloading the remote ZIP is made.
- General push-triggered CI run `29621464929` also completed `success` at exact
  workflow commit `7038030d07c9eb1b76e0af2a3fdc84154d9b6fe2`.
- The RC.2 remote prerequisites are complete. EXT-20 remains unchecked. The
  next bounded pass is to update and verify the external Level-1 suite against
  immutable RC.2, prepare a new collision-free no-model run root, and stop
  before the separately confirmed live real-Codex invocation. No tag or
  external-use approval was created.

## EXT-20 RC.2 Remote Artifact Attestation Workflow (2026-07-17)

- Task selected: the bounded RC.2 remote-artifact prerequisite of `EXT-20`
  only. A new separate
  `.github/workflows/level1-rc2-candidate-attestation.yml` triggers only on a
  push to `level1-v0.1.0-rc.2-attestation` and checks out exact candidate
  source commit `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec` rather than the
  trigger HEAD.
- The workflow installs exact Go 1.26.5 with cache restoration disabled, makes
  two independent clean `--no-local` source clones with separate Go build and
  module caches, and builds Linux, Darwin, and FreeBSD amd64 artifacts with
  the settled EXT-18 environment, build flags, empty build ID, and
  `main.version=0.1.0`. It compares each pass pair byte-for-byte and requires
  the exact RC.2 SHA-256 values
  `06c1258a947def8c53e03bfd79944bb002351358fc8dfecd35682ab7532b5010`,
  `05a15786dd1617d77ec671f420075922f6f9a78bf03de1245f03008f0960dee1`,
  and `5891c88e1e13f5a0a0e3452c15221981a187652c2e563a7b8b218b63c07d2a29`.
- Every artifact must expose the exact Go toolchain, command path, compiler,
  trimpath, target, disabled CGO, Git source revision, and
  `vcs.modified=false` build metadata. Each artifact must also carry exactly
  one `main.version` symbol and exact `0.1.0` string; both Linux copies must
  execute with exact output `revolvr 0.1.0`. One upload retains both binary
  sets, all six build-metadata/version assertions, `SHA256SUMS`, pairwise
  reproducibility hashes, and the exact tabular authority manifest.
- Files changed: the new RC.2 workflow and this state file only.
  `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked, and
  `.agent/DECISIONS.md` remains unchanged because this workflow implements the
  already settled RC.2 authority. The RC.1 workflow remains byte-for-byte
  unchanged at SHA-256
  `d1314182a0cffd78927e6a5cc688e370c42f3d17a4e4ffe426f647a384c40a41`;
  no RC.1 ref, hash, bundle, or failed live evidence changed.
- Verification commands run: all required durable-state reads; raw Git status,
  history, exact candidate object/tree, and local/remote ref-collision checks;
  PyYAML BaseLoader parsing with exact trigger/checkout/toolchain/build/hash/
  metadata/upload assertions; `bash -n` on the embedded workflow shell; and
  two executions of the actual embedded shell against fresh detached clones
  under host Go 1.26.5. The first execution completed all builds and hash
  checks but its outer local harness miscounted the retained files; the single
  repair corrected that harness assertion from 18 to 21. The complete rerun
  passed all six required hashes, all three byte-for-byte pair comparisons,
  every retained-authority assertion, and `git diff --check`.
- Verification result: local workflow syntax, exact constants, clean two-pass
  execution, metadata, version, hashes, reproducibility, and retained evidence
  passed. Independent controller verification parsed the workflow authority,
  syntax-checked and executed its exact embedded shell from a detached clean
  checkout, then verified all 21 retained files, six SHA-256 entries, three
  byte-identical artifact pairs, and the complete authority manifest. No
  remote workflow, live/nested Codex, model, commit, push, or tag operation was
  started during either local verification pass.
- Collision-free raw-Git attestation ref to publish after controller review
  and commit: `refs/heads/level1-v0.1.0-rc.2-attestation`. Raw Git found that
  ref absent both locally and at `origin`; publication should use the reviewed
  workflow commit as its tip and must not move the exact candidate ref.
- Completion is recorded in `EXT-20 RC.2 Remote Artifact Attestation Result`
  above. A new collision-free RC.2 EXT-20 real-Codex suite may now be prepared,
  but live model execution still requires its separate explicit confirmation.
  External use remains unapproved; blocker for the next local subtask: none.

## EXT-20 RC.2 Exact-Candidate Remote CI (2026-07-17)

- Independent controller verification passed for the local RC.2 bundle and a
  third clean-clone rebuild. Raw Git published collision-free branch
  `level1-v0.1.0-rc.2` at exact source commit
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`; remote readback resolves that
  ref to the same SHA.
- Push-triggered GitHub Actions CI run `29620366441` completed `success` on
  that exact SHA. All ten jobs passed: Go 1.22 source floor and tests; Darwin,
  Linux, and FreeBSD amd64 builds; Windows diagnostic stub; vet and module
  verification; fake-Codex success and verification-failure smokes; race
  tests; and the production autonomous strict-fake suite. Run evidence:
  `https://github.com/ponchione/revolvr/actions/runs/29620366441`.
- The local evidence update was committed as `916c856`; raw Git pushed `main`
  and the exact candidate ref. No `gh`, tag, attestation workflow, live Codex,
  or external-use approval was used or created.
- EXT-20 remains unchecked. The next bounded pass is the separate RC.2 remote
  artifact attestation workflow only. It must check out exact `eeaaf50`, pin Go
  1.26.5, reproduce both clean build passes and all three recorded hashes,
  verify embedded metadata, retain the attested artifact bundle, and preserve
  every RC.1 workflow/evidence unchanged.

## EXT-20 RC.2 Replacement Candidate — Local Verification (2026-07-17)

- Task selected: the bounded replacement-candidate subtask of `EXT-20` only.
  Candidate `level1-v0.1.0-rc.2` binds release version `0.1.0` to exact clean
  source commit `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec` and tree
  `f1eef2999103ffdc9b76fceda34af824191dd4b7`. Local and remote ref checks plus
  local bundle/root checks found no prior RC.2 collision. RC.1, both retired
  EXT-20 live roots, their remote refs, and their evidence were not changed.
- The production omitted-work-directory fix was inspected first. Its focused
  regression passed on Go 1.26.5, under the race detector, and with the full
  `internal/cli` package before candidate construction; the focused ordinary
  and race results are also retained with the release verification evidence.
- The immutable candidate bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.2-eeaaf50b52fd/`.
  Its build-instruction SHA-256 is
  `c9e38a38684c5022445fbc59725f4ecd4f7d143540ad2bb99a3aa8c208f2bdcf`
  and its complete inventory SHA-256 is
  `d398e5ae9a2a74965ad76134b48311e593d299a0c1003e7e2f19b72a74f1c0e7`.
  Two independent non-local clean clones produced byte-identical artifacts:
  Linux amd64
  `06c1258a947def8c53e03bfd79944bb002351358fc8dfecd35682ab7532b5010`,
  Darwin amd64
  `05a15786dd1617d77ec671f420075922f6f9a78bf03de1245f03008f0960dee1`,
  and FreeBSD amd64
  `5891c88e1e13f5a0a0e3452c15221981a187652c2e563a7b8b218b63c07d2a29`.
- Every artifact independently records Go 1.26.5, its exact target,
  `CGO_ENABLED=0`, source revision `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`,
  and `vcs.modified=false`; the Linux binary reports `revolvr 0.1.0`. The
  sibling immutable verification bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.2-eeaaf50b52fd-verification/`.
  It retains exact source/Go/Git/govulncheck identities, commands, logs,
  independent build metadata, both build-attempt logs, and a complete
  inventory with SHA-256
  `45a6d14e90aaf95de7f1a67e0179edf50142e226ecdbc52259e8c60c77531c9c`.
- Required verification passed: Go 1.22.12 source-floor tests; Go 1.26.5 full
  tests; `go vet ./...`; `go mod verify`; `govulncheck ./...` and verbose scan;
  supported Linux/Darwin/FreeBSD amd64 builds; exact duplicate-build hash
  comparison; embedded version/source/toolchain checks; candidate self-check;
  verification-bundle inventory check; and source cleanliness. Govulncheck
  found zero reachable and zero imported-package vulnerabilities. Its one
  retained module-only finding is `GO-2026-5024` in the Windows-only
  `golang.org/x/sys/windows` surface at `v0.30.0`; Revolvr does not call it and
  the report names `v0.44.0` as fixed.
- Independent controller verification reran both bundle inventories, rejected
  links/aliases, checked every artifact hash and embedded build field, verified
  RC.1 remained intact, and ran the RC.2 Linux binary from a retired external
  repository. RC.2 passed repository/external admission and refused only an
  intentionally absent task at scheduler selection, with no operation or Git
  mutation. A third build from another clean non-local clone reproduced all
  three artifact hashes exactly. Its complete bundle inventory differs because
  the first line of each retained `go version -m` text records that rebuild's
  absolute artifact pathname; this documented path-only text variance is not
  artifact or embedded-metadata authority. The candidate binaries and all
  embedded fields remain byte-identical to the authoritative bundle.
- The first construction used a relative output path. Artifacts reproduced,
  but fail-closed verification rejected the path-bearing build metadata. That
  immutable diagnostic is retained at
  `.revolvr/release-candidates/level1-v0.1.0-rc.2-eeaaf50b52fd.failed-relative-metadata-path/`.
  The single repair used the settled absolute output-path invocation and
  passed every check; no failed bundle was overwritten.
- Files changed: this state file, `.agent/DECISIONS.md`, the new ignored RC.2
  pinned build script, candidate bundle, verification bundle, and failed
  diagnostic. No Go source, dependency, RC.1 evidence, failed EXT-20 root,
  commit, push, tag, remote ref, workflow run, or live/model operation changed.
  `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked.
- Exact remaining remote-attestation step: after independent controller
  verification, use raw Git to publish a collision-free
  `level1-v0.1.0-rc.2` ref at exact source
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`; require every EXT-15 CI job on
  that SHA to pass. Then publish a separate RC.2 attestation-workflow commit
  that checks out that immutable source, pins Go 1.26.5 and the three hashes
  above, rebuilds both clean passes, verifies embedded metadata, and retains a
  remote artifact digest. Compare its outputs byte-for-byte with this local
  bundle before preparing a new collision-free EXT-20 live suite. No `gh`,
  push, remote CI request, or attestation was performed in this pass.
- Result: local replacement-candidate construction passed. What remains is
  fresh remote exact-commit CI/artifact attestation followed by the complete
  quantitative real-Codex gate. Blockers for this bounded local task: none.

## EXT-20 Candidate Rejected: CLI Work Directory (2026-07-17)

- The second live attempt reached the exact RC.1 binary but stopped before
  supervisor/model admission. `revolvr run --until-terminal` returned
  `inspect repository paths: harness runtime path: repository root is required`
  for operation `ext20-579a7316af31-01`; the collector consequently retained
  an incomplete, non-manifest diagnostic tree at
  `/tmp/revolvr-ext20-live2.3ZVQcm/suite/evidence/repo-a/01-successful-source-change-1`.
  No task-operation directory, run, receipt, source change, commit, or model
  invocation exists. The suite root is retired because incomplete evidence is
  collision authority and must not be overwritten.
- Root cause: the production entry point constructs `cli.Options` without
  `WorkDir`. Read-oriented commands normalize an empty work directory through
  `resolveStatePaths`, but autonomous `run` passes it directly to external
  admission. The immutable RC.1 candidate therefore cannot start any real
  attended task, even when its process current directory is the repository.
- Fix: `cli.NewRootCommand` now resolves an omitted work directory from
  `os.Getwd` once before constructing subcommands. A CLI regression test calls
  autonomous run with production-style empty options and proves the exact
  current directory reaches `app.Config`.
- Verification passed: `gofmt`; focused CLI, app, repository-path, and runtime-
  path tests; `go test -count=1 ./...`; `git diff --check`; and a newly built
  production CLI invoked from the pristine second external repository. The
  smoke command passed repository/external admission and refused only the
  intentionally absent task at scheduler selection, created no operation, and
  left Git clean.
- Release consequence: candidate RC.1 at source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f` is rejected for Level-1 external
  use. Its EXT-18/EXT-19 evidence remains immutable historical evidence but
  cannot satisfy EXT-20. A replacement candidate must be built reproducibly
  from the work-directory fix, receive fresh local/remote attestation, and use
  a new collision-free EXT-20 suite. The old candidate binary, bundles, remote
  refs, artifacts, and failed roots must not be edited or relabeled.
- EXT-20 remains unchecked. The next fresh bounded pass is replacement local
  candidate construction and verification only; it must not start live Codex,
  push, tag, or mark external use approved.

## EXT-20 Live Preflight Bound-Rendering Fix (2026-07-17)

- The first confirmation-gated live command stopped during EXT-17 collector
  admission for operation `ext20-e92c1bdec435-01` with status 1 and no terminal
  manifest. The retained diagnostic is
  `/tmp/revolvr-ext20-fresh.OmCBwv/suite/logs/01-successful-source-change-1.stderr`.
  No Codex/model process, task operation, runtime evidence, source change, or
  aggregate file was started; both disposable repositories remained clean.
- Root cause: `scripts/dogfood-external-level1.sh` still asserted the obsolete
  strict-fixture rendering `audit:4`, `elapsed_nanoseconds`, and
  `process_nanoseconds`. The exact EXT-18 candidate renders its settled public
  config contract as `action_attempts=[audit=4,...]`, `elapsed=4h0m0s`, and
  `process_duration=30m0s`, so the collector rejected the valid release before
  model admission.
- Fix: the collector fixture now reproduces the exact release rendering and
  real-operation preflight compares the complete canonical attended-bounds
  line byte-for-byte. This removes the fixture/production drift while retaining
  fail-closed validation of every Level-1 bound.
- Verification passed: shell syntax; two fresh fixture-only collections and
  independent manifest verification; wrong-Codex preflight rejection; suite
  static verification; and a real-candidate/no-model preflight probe that
  passed candidate, Codex, config, and exact bounds authority before refusing
  only the intentionally absent task at doctor. The probe created no evidence
  and left Git clean.
- The failed preparation root is retired to preserve its diagnostic. A fresh
  no-model suite is prepared at `/tmp/revolvr-ext20-live2.3ZVQcm/suite` with
  zero manifests, an empty aggregate directory, two clean repositories, and
  exact isolated `codex-cli 0.144.4`. Live confirmation refusal and pre-live
  aggregate refusal both passed without mutation.
- EXT-20 remains unchecked. Resume with:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-live2.3ZVQcm/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`.

## EXT-20 Guarded Level-1 Suite Driver (2026-07-17)

- Task selected: `EXT-20`, implement the guarded driver for the complete
  quantitative Level-1 real-Codex dogfood suite without starting live model
  work in this pass.
- `scripts/dogfood-external-level1-suite.sh` now has four explicit modes:
  no-write/no-model `--static`, no-model `--prepare`, confirmation-gated
  `--live`, and independent `--verify-suite`. Live execution is impossible
  without the exact value
  `EXT20_LIVE_REAL_CODEX_MODEL_CALLS` supplied to
  `--confirm-live-real-codex`.
- Preparation either installs isolated `@openai/codex@0.144.4` or accepts an
  explicit isolated npm prefix, then requires exact output
  `codex-cli 0.144.4` and SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  It separately verifies the complete immutable EXT-18 bundle and Linux
  candidate SHA-256
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`
  at source commit `ed65049fba6bf82852fd406ebc17afa90a953e3f`.
- The prepared plan contains 11 unique operations across two new disposable
  repositories: five named successful source changes, a completed correction
  with final verification and re-audit, a retained production verification
  failure, needs input, graceful cancellation followed by a new-operation
  restart, and a hostile-instruction supervisor safety refusal. Every
  operation invokes `scripts/dogfood-external-level1.sh` once. Existing
  terminal bundles are verified and reused; incomplete path collisions fail
  without overwrite.
- Aggregate verification independently verifies every EXT-17 manifest,
  outside-sentinel equality, control-HEAD containment, candidate/Codex/config
  identity, terminal operation/history, every retained ledger/receipt
  validation result, attempt charge equality, exact task-branch commit counts,
  unique commit heads, all quantitative thresholds, and every zero-tolerance
  counter. Its sorted operation table and report are deterministic and
  hash-listed; conflicting retained rows, report, or checksum authority are
  never overwritten. Failed verification removes its unpublished temporary
  aggregate files. The driver never edits task-run, state, history, receipt,
  recovery, or other live runtime evidence.
- Controller review found and repaired two evidence-quality defects before any
  live call: failed pre-live verification retained an unpublished aggregate
  temporary file, and aggregate verification did not independently inspect
  every hashed collector ledger/receipt result. It also narrowed permitted
  control-root metadata changes to the selected task, checkpoints them after
  every terminal outcome, makes retained-bundle replay recheck exact task and
  outcome authority, and counts only the five explicit successful-source
  scenarios toward that threshold.
- Files changed in this pass:
  `scripts/dogfood-external-level1-suite.sh`, `agent-ext20.sh`, and this state
  file. The helper preserves the fresh-session implementation command used to
  create the driver; the live model suite remains a separate confirmation-
  gated command.
  `.agent/TASKS.md` remains unchanged with EXT-20 unchecked, and
  `.agent/DECISIONS.md` remains unchanged because this applies the settled
  EXT-17 through EXT-20 evidence authority without changing architecture.
- Verification commands run: all required durable-state reads; shell syntax
  checks for the new driver and EXT-17 collector; the driver's `--static`
  mode; the pinned candidate bundle `--verify` path; a fresh standalone
  no-model `--prepare --install-codex` run; a second no-model preparation using
  the accepted-prefix path; exact installed package/version/executable hash
  inspection; clean Git/status and exact-task doctor checks for both prepared
  repositories; refusal of live mode without the confirmation value; refusal
  of a colliding preparation root; refusal of pre-live aggregate validation
  with no manifests and no aggregate publication; independent controller
  syntax/static review; confirmation-refusal testing; source scans for nested
  Codex, `gh`, push, and source-runtime edits; `go test ./...`; focused CLI and
  task-run persistence/cancellation tests; and `git diff --check`. One
  initial preparation found a same-line Bash `local` expansion under
  `set -u`; the single repair split the derived assignment, after which both
  preparation forms and the complete static verification passed. A later
  attempt to remove two temporary preparation-check roots was rejected before
  execution by the command guard and changed no repository or suite evidence.
- Verification result: implementation and no-model preparation passed. The old
  preparation root was retired after it retained a temporary file produced by
  the pre-repair verifier. A fresh standalone prepared suite with zero
  operation manifests and an empty aggregate directory is retained at
  `/tmp/revolvr-ext20-fresh.OmCBwv/suite`; both external repositories are
  clean, every task doctor reported ready during preparation, and its isolated
  Codex path reports the exact required version and digest.
- What remains: run the real suite with the unmistakable explicit
  confirmation, retain all 11 terminal bundles, and independently validate
  the aggregate:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-fresh.OmCBwv/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. EXT-20 must remain unchecked until this
  command finishes and `--verify-suite` passes every retained manifest and
  threshold.
- Blockers: this implementation pass explicitly forbids live or nested Codex
  operations, so it cannot produce qualifying manifests. No live model call,
  current-repository commit, push, tag, or remote mutation was started.

## EXT-20 Real-Codex Dogfood Gate Blocked (2026-07-17)

- Task selected: `EXT-20`, execute and independently validate the quantitative
  Level-1 real-Codex dogfood gate for the exact release candidate.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-20 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: all required durable-state reads; `git status
  --branch --porcelain=v2`; exact HEAD and remote candidate-ref inspection;
  candidate-bundle inventory, version, SHA-256, and Go build-metadata checks;
  the pinned candidate bundle `--verify` command; installed Codex path,
  SHA-256, and version inspection; the embedded release-manifest inspection;
  searches for dogfood manifests under the repository, `/tmp`, and
  `/home/gernsback`; inspection of every discovered intact manifest; current
  collector `--verify-manifest` for all four intact bundles; and `git diff
  --check`.
- Verification result: blocked. The Linux candidate remains intact at exact
  SHA-256
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  reports `revolvr 0.1.0`, and records source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f`. The only discovered Level-1
  manifests are four valid EXT-17 fixture bundles whose manifests explicitly
  record `fixture_only\ttrue`, the synthetic candidate, and synthetic Codex
  identity. They prove the collector mechanism but contribute zero real-Codex
  operations, repositories, successful source changes, or required production
  scenarios to EXT-20.
- What remains: collect at least ten qualifying manifests from real-Codex task
  operations across at least two disposable external repositories, including
  five successful source changes plus verification failure/correction, needs
  input, cancellation/restart, and safety refusal. Then verify every manifest,
  total the independent thresholds, compare Git and outside-sentinel
  authority, and validate every ledger and receipt before completing EXT-20.
- Blockers: this pass explicitly forbids starting a nested Codex run, so it
  cannot generate the missing real-operation evidence. In addition, the only
  installed Codex reports `codex-cli 0.144.5`; the candidate's embedded
  release manifest admits exactly `codex-cli 0.144.4` with the observed
  executable SHA-256, so current admission would reject it before model work.
  No model, repository fixture, operation, commit, push, tag, or remote
  mutation was started.

## EXT-19 Exact Candidate Remote CI And Artifact Attestation (2026-07-17)

- Task selected: `EXT-19`, push the exact Level-1 candidate and obtain remote
  CI plus tested-binary evidence.
- Under the operator's explicit raw-Git publication authorization, remote
  branch `level1-v0.1.0-rc.1` names exact candidate source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f`. Push-triggered CI run
  `29612464054` completed successfully on that SHA; all ten mandatory EXT-15
  jobs succeeded.
- Supplemental workflow commit
  `a1afdd73a7bfb03e9e5ef361616604115f9db5b8` is published at remote branch
  `level1-v0.1.0-rc.1-attestation`. It checks out the immutable candidate SHA,
  uses Go 1.26.5 and the exact EXT-18 release commands, verifies embedded
  version/source metadata, and compares the Linux, Darwin, and FreeBSD amd64
  binaries with the recorded EXT-18 SHA-256 values.
- Attestation run `29615752091` completed successfully and retained artifact
  `level1-v0.1.0-rc.1-attestation`, size 35,090,832 bytes, with GitHub artifact
  digest
  `sha256:def158256b667447248a0370ee6e2dbe724b2dc1971216300e21751d706ff94f`.
  The artifact is not expired. Its run URL is
  `https://github.com/ponchione/revolvr/actions/runs/29615752091`; the candidate
  CI URL is `https://github.com/ponchione/revolvr/actions/runs/29612464054`.
- Controller verification used raw `git` for local and remote refs and the
  official read-only GitHub Actions REST projections for run, job, and artifact
  conclusions. No `gh`, tag, merge, rebase, or Revolvr publication operation
  was used.
- Verification result: the remote candidate identity, complete required CI
  matrix, exact release-binary hashes, and retained artifact evidence all
  passed. EXT-19 is complete.
- What remains: `EXT-20`, the quantitative Level-1 real-Codex dogfood gate.
  External-project use remains unapproved pending EXT-20 and EXT-21.
- Blockers for EXT-19: none.

## EXT-19 Supplemental Candidate Attestation — Fresh Local Verification (2026-07-17)

- Task selected: `EXT-19`, add the smallest supplemental remote attestation
  workflow for exact Level-1 candidate commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f` while leaving completion gated on
  its remote result.
- The already-present
  `.github/workflows/level1-candidate-attestation.yml` requires no repair. It
  triggers only on pushes to `level1-v0.1.0-rc.1-attestation`, checks out the
  exact candidate commit, installs Go 1.26.5, reproduces the EXT-18 Linux,
  Darwin, and FreeBSD amd64 command/environment/version metadata, compares the
  three recorded SHA-256 values, and uploads the binaries, metadata, and
  `SHA256SUMS` as one artifact.
- Files changed in this pass: this state file only. The workflow remains
  unchanged, `.agent/TASKS.md` remains unchanged with EXT-19 unchecked, and
  `.agent/DECISIONS.md` remains unchanged because no implementation or
  architecture authority changed. Candidate source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f` remains the exact immutable build
  source.
- Verification commands run: all required durable-state reads; raw `git`
  worktree, history, candidate-object, ancestry, and workflow inspection;
  PyYAML BaseLoader parsing plus exact trigger/checkout/toolchain/build/hash/
  artifact assertions; host `go1.26.5` verification; execution of the actual
  embedded workflow shell block in a fresh detached clone of the candidate;
  hash-manifest verification; the pinned EXT-18 bundle `--verify` command; and
  `git diff --check`. An initial command wrapper was rejected before execution
  because of its temporary-cleanup spelling; the same check was rerun with an
  exact temporary path and completed successfully without repository changes.
- Verification result: local workflow structure and execution passed. The
  rebuilt Linux, Darwin, and FreeBSD amd64 artifacts matched the recorded
  EXT-18 SHA-256 values
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  `1c28e844196e88dd03daffde2a24a417d88571ab31bba2b022438b9453aa9fdb`,
  and `8b7860b801e30f7d36258cde1da4a8af5e9cb312177bd46fc0003a439fca0e17`.
  The uploaded-file projection contained the three binaries, their build
  metadata, Go/source/version metadata, and `SHA256SUMS`; the complete EXT-18
  bundle also reverified.
- What remains: the controller must verify the workflow commit/ref, confirm a
  successful remote attestation job, and record its run URL, conclusion, and
  retained artifact evidence. Only then may EXT-19 be marked complete.
- Blockers: this local pass does not establish the remote workflow conclusion
  or artifact retention. No commit, push, tag, `gh`, or nested Codex operation
  was used.

## EXT-19 Supplemental Candidate Attestation Workflow (2026-07-17)

- Task selected: `EXT-19`, push the exact Level-1 candidate and obtain remote
  CI evidence. This pass implements only the operator-directed supplemental
  remote binary attestation workflow and does not publish it.
- `.github/workflows/level1-candidate-attestation.yml` triggers only on a push
  to `level1-v0.1.0-rc.1-attestation`, explicitly checks out candidate source
  commit `ed65049fba6bf82852fd406ebc17afa90a953e3f`, installs Go 1.26.5,
  rebuilds Linux/Darwin/FreeBSD amd64 with the exact EXT-18 environment and Go
  flags, validates and retains `go version -m` evidence, compares all three
  SHA-256 values with the recorded EXT-18 values, and uploads the binaries,
  metadata, and `SHA256SUMS` as one workflow artifact.
- Files changed in this pass:
  `.github/workflows/level1-candidate-attestation.yml` and this state file.
  `.agent/TASKS.md` remains unchanged and EXT-19 remains unchecked.
  `.agent/DECISIONS.md` remains unchanged because the workflow directly
  implements already-settled release authority rather than changing it. The
  pre-existing untracked `agent-ext19.sh` was not modified.
- Verification commands run: the required durable-state reads; worktree and
  exact candidate inspection using raw `git`; PyYAML BaseLoader parsing plus
  assertions for the exact trigger, checkout, toolchain, build flags, metadata
  checks, recorded hashes, and artifact upload; execution of the workflow's
  extracted build-and-attest shell block in a fresh detached clone of the
  candidate commit under host Go 1.26.5; the pinned EXT-18 bundle `--verify`
  command; untracked-workflow whitespace validation; and `git diff --check`.
- Verification result: local workflow structure and execution passed. The
  fresh candidate rebuild produced exact EXT-18 SHA-256 values for Linux
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  Darwin `1c28e844196e88dd03daffde2a24a417d88571ab31bba2b022438b9453aa9fdb`,
  and FreeBSD
  `8b7860b801e30f7d36258cde1da4a8af5e9cb312177bd46fc0003a439fca0e17`.
  The complete immutable EXT-18 bundle also reverified.
- What remains: the controller must verify, commit, and push the supplemental
  workflow to the exact trigger branch, wait for its remote job to succeed,
  and record the run URL, conclusion, and retained artifact evidence. EXT-19
  must remain unchecked until that remote workflow passes.
- Blockers: the required remote workflow has not run because this pass is
  expressly prohibited from committing or pushing. No commit, push, tag,
  `gh`, or nested Codex operation was used.

## EXT-19 Remote CI Observed — Completion Still Blocked (2026-07-17)

- Task selected: `EXT-19`, push the exact Level-1 candidate and obtain remote
  CI evidence.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-19 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: the required durable-state reads; `git status
  --branch --porcelain=v2`; `git rev-parse --verify HEAD`; `git branch
  --show-current`; `git remote -v`; `git branch -vv`; recent `git log`; exact
  candidate/evidence commit inspection; `command -v gh`; `git ls-remote
  --heads origin`; read-only GitHub commit, workflow, and status connector
  queries; the public GitHub Actions run, jobs, and artifacts API projections;
  `.github/workflows/ci.yml` inspection; local candidate manifest and artifact
  hashing; the candidate bundle's pinned `--verify` command; and `git diff
  --check`.
- Verification result: remote branch `level1-v0.1.0-rc.1` points to exact
  candidate source commit `ed65049fba6bf82852fd406ebc17afa90a953e3f` while
  remote `main` remains `e76280cc93404aab403f8fe34036e6971e58bb78`.
  Push-triggered CI run `29612464054` on the candidate commit completed on its
  first attempt with conclusion `success`; all ten required jobs and their
  exact-source assertions succeeded. The run URL is
  `https://github.com/ponchione/revolvr/actions/runs/29612464054`.
- The immutable EXT-18 bundle still verifies. Its Linux, Darwin, and FreeBSD
  amd64 SHA-256 values remain
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  `1c28e844196e88dd03daffde2a24a417d88571ab31bba2b022438b9453aa9fdb`,
  and `8b7860b801e30f7d36258cde1da4a8af5e9cb312177bd46fc0003a439fca0e17`.
- What remains: obtain direct operator authorization or an explicit operator
  confirmation naming the already-published exact commit and target ref. Also
  obtain remote evidence that hashes the binaries actually tested by CI and
  compares them with EXT-18. This run retained zero workflow artifacts, and
  the candidate workflow builds only into runner-temporary paths without
  hashing or uploading those outputs, so that required comparison cannot be
  reconstructed from run `29612464054`.
- Blockers: this pass contains no explicit commit/push authorization, and
  EXT-19 expressly forbids completion without it; remote state alone is not
  authorization. The required tested-binary hash evidence is also absent. The
  `gh` executable is unavailable, though the official read-only Actions API
  exposed the run and job conclusions. No push, commit, tag, workflow rerun,
  or other remote mutation was attempted.

## EXT-19 Remote CI Gate Blocked — Missing Push Authorization (2026-07-17)

- Task selected: `EXT-19`, push the exact Level-1 candidate and obtain remote
  CI evidence.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-19 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: `git status --branch --porcelain=v2`; `git
  rev-parse --verify HEAD`; `git branch --show-current`; `git remote -v`;
  `git branch -vv`; recent `git log`; `git show --stat --summary` for candidate
  source commit `ed65049fba6bf82852fd406ebc17afa90a953e3f` and evidence commit
  `413c3f11053f8d04e2aca10c5d5d33d38078ae29`; `git diff --check`; untracked
  file inspection; `git ls-remote --heads origin`; an attempted `gh auth
  status`/workflow query; and read-only GitHub connector checks for repository,
  branch, candidate-commit, and candidate-workflow authority.
- Verification result: blocked before publication. The local tree is clean at
  `413c3f11053f8d04e2aca10c5d5d33d38078ae29` on `main`, two commits ahead of
  `origin/main`. The ignored release artifacts are bound to exact source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f`; the later local commit records
  EXT-18 evidence only. GitHub has only `main` at
  `e76280cc93404aab403f8fe34036e6971e58bb78`, reports the candidate source
  commit absent, and has no workflow run for it. The required `gh` executable
  is also unavailable in this environment.
- What remains: obtain direct operator authorization naming the exact push and
  target ref, publish the candidate source commit without using Revolvr, wait
  for the complete EXT-15 workflow on that exact commit, inspect every required
  job, compare tested artifact hashes with EXT-18, and record the CI URL and
  conclusions in release-decision evidence.
- Blocker: this pass contains no explicit commit/push authorization, and EXT-19
  expressly forbids completion without it. Pushing `main` as-is would test the
  later evidence commit rather than make the EXT-18 source commit the pushed
  ref tip, so the target ref must be explicit. No push, commit, tag, workflow
  rerun, or remote mutation was attempted.

## EXT-18 Reproducible Level-1 Release Candidate (2026-07-17)

- Task selected: `EXT-18`, produce a reproducible, versioned Level-1 release
  candidate from one clean exact source commit.
- Candidate `level1-v0.1.0-rc.1` uses release version `0.1.0`, exact source
  commit `ed65049fba6bf82852fd406ebc17afa90a953e3f`, and the current official
  stable patched toolchain `go1.26.5`. The exact future artifact is versioned
  now so Level-1 dogfood tests the bytes eligible for the later `v0.1.0`
  decision rather than a differently versioned development build.
- The ignored local bundle at
  `.revolvr/release-candidates/level1-v0.1.0-rc.1-ed65049fba6b/` contains the
  pinned build instructions, Linux/Darwin/FreeBSD amd64 artifacts, `go version
  -m` evidence, byte-for-byte duplicate-build comparison, canonical candidate
  manifest, complete SHA-256 inventory, and inventory digest. Its inventory
  SHA-256 is
  `7a87c571f59a758fcf979acd9980e9799fda0a0c06bc9be4dce8ca44f37b1dde`;
  build-instruction SHA-256 is
  `6e1782dedfd56b6e0ac4350d35a8379650ac2aa7af34e1f1b41057272cae9b84`.
- Artifact SHA-256 values are Linux amd64
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  Darwin amd64
  `1c28e844196e88dd03daffde2a24a417d88571ab31bba2b022438b9453aa9fdb`,
  and FreeBSD amd64
  `8b7860b801e30f7d36258cde1da4a8af5e9cb312177bd46fc0003a439fca0e17`.
  Both independent clean-clone builds produced those exact hashes.
- The sibling `-verification` bundle retains source-floor, host, vet, module,
  vulnerability, and candidate-verification logs under complete read-only
  SHA-256 inventory. Its inventory SHA-256 is
  `0dcccc3ae6051791fd10effeac754ad2d5c6dcdc8b61fd0900e3c862aefe68f2`.
  `govulncheck` found zero reachable and zero imported-package
  vulnerabilities. Its one separately retained module-only finding is
  `GO-2026-5024` in `golang.org/x/sys@v0.30.0`, a Windows-only
  `NewNTUnicodeString` symbol Revolvr does not call; the report names v0.44.0
  as fixed.
- Files changed in this pass: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/DECISIONS.md`; ignored local candidate instructions, artifacts, and
  verification evidence were added under `.revolvr/release-candidates/`. No
  Go source, dependency, commit, push, tag, or real Codex operation changed.
  The first generated bundle failed its own verification because its recorded
  `go version -m` filename named a temporary artifact. The one permitted
  repair records metadata from the final installed artifact; the failed
  diagnostic bundle is retained with suffix `.failed-metadata-path`.
- Verification commands: clean-source Git admission; official Go release
  inspection; `bash -n` on the pinned instructions; two independent clean
  builds of every supported target; bundle inventory/metadata/version
  verification; `go version`; `GOTOOLCHAIN=go1.22.12 go version`;
  `GOTOOLCHAIN=go1.22.12 go test -count=1 ./...`; `go test -count=1 ./...`;
  `go vet ./...`; `go mod verify`; `govulncheck ./...`; `govulncheck -show
  verbose ./...`; and complete verification-evidence inventory checks.
- Verification result: all required source-floor, host, static, module,
  vulnerability, supported-build, reproducibility, metadata, and inventory
  checks passed after the single metadata-path repair.
- What remains: `EXT-19`, push this exact candidate commit and obtain remote
  CI evidence. That task must not push without direct operator authorization.
  External-project use remains unapproved pending EXT-19 through EXT-21.
- Blockers for EXT-18: none.

## EXT-18 Release Candidate Blocked — Fresh Recheck (2026-07-17)

- Task selected: `EXT-18`, produce a reproducible, versioned Level-1 release
  candidate from one clean exact source commit.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-18 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: `git status --branch --porcelain=v2`; `git
  rev-parse --verify HEAD`; `git describe --tags --always --dirty`; `git diff
  --stat`; and `git ls-files --others --exclude-standard`.
- Verification result: blocked before release construction. `HEAD` remains
  exact commit `e76280cc93404aab403f8fe34036e6971e58bb78`, but the candidate
  source remains outside that commit as 45 tracked-file modifications plus
  untracked EXT-01 through EXT-17 release files. `git describe` reports
  `e76280c-dirty`, so duplicate builds, candidate hashes, vulnerability
  conclusions, supported-platform artifacts, and embedded source metadata
  would not be authoritative release evidence.
- What remains: obtain direct operator authorization to review and commit the
  complete intended source tree as one exact candidate source commit, then run
  EXT-18 in a fresh pass to finalize immutable build instructions, build twice
  in fresh directories, compare hashes, run the full test/vet/module/
  vulnerability/platform matrix, and verify embedded version/source metadata.
- Blocker: this pass explicitly forbids commits. Making the tree clean would
  require committing the intended release source or destructively discarding
  or hiding repository changes; neither action is authorized. No repair was
  attempted because every available repair would violate the pass rules or
  destroy user-owned work.

## EXT-18 Release Candidate Blocked (2026-07-17)

- Task selected: `EXT-18`, produce a reproducible, versioned Level-1 release
  candidate from one clean exact source commit.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-18 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: `git status --short`; `git status --branch
  --porcelain=v2`; `git rev-parse HEAD`; `git describe --tags --always
  --dirty`; `git tag --list --sort=-version:refname`; and a focused source scan
  for existing version, build-metadata, and reproducibility surfaces.
- Verification result: blocked before candidate construction. The checkout is
  based on exact commit `e76280cc93404aab403f8fe34036e6971e58bb78` but has
  dozens of tracked modifications and untracked files containing the completed
  EXT-01 through EXT-17 work. It is therefore not the clean exact source
  commit required by EXT-18, and no candidate hash, duplicate build,
  vulnerability conclusion, supported-platform artifact, or embedded metadata
  claim can be authoritative for this tree.
- What remains: with direct operator commit authorization, commit the complete
  intended source tree as one reviewed candidate source commit, then run EXT-18
  in a fresh pass to add or finalize immutable build instructions, build twice
  in fresh directories, compare hashes, run the full test/vet/module/
  vulnerability/platform matrix, and verify embedded version/source metadata.
- Blocker: this pass forbids commits. No repair was attempted because making
  the tree clean would require either committing the intended release source or
  destructively discarding/hiding repository changes; neither action is
  authorized.

## EXT-17 Wrong-Codex Refusal Oracle Repair (2026-07-17)

- Task selected: `EXT-17`, repair and freshly verify the opt-in Level-1
  dogfood evidence collector after the wrong-Codex no-mutation oracle failed
  nondeterministically.
- The failure was reproduced three times in 30 runs. `stat_fields` emitted
  literal `\\t` text, so the Git index projection's `cut -f1-3` retained its
  volatile mtime. A read-only status refresh crossing a one-second boundary
  then appeared to change Git authority even though the index bytes, mode,
  size, and link count were unchanged. Stat fields now contain real tab
  delimiters, preserving the intended metadata while excluding only index
  mtime from semantic Git authority. Any future Git projection mismatch prints
  the exact recursive diff before the temporary diagnostics are removed.
- Files changed in this fresh pass:
  `scripts/dogfood-external-level1.sh`, `.agent/TASKS.md`, and this state file.
  No dependency, production Go code, durable architecture decision, or commit
  was added.
- Verification commands: `bash -n scripts/dogfood-external-level1.sh`; 50
  consecutive wrong-Codex fault repetitions; two independent `--fixture-only`
  collections; canonical manifest comparison after removing only
  `collected_at_utc`; `--verify-manifest` for both bundles; all four dirty,
  non-disposable, wrong-binary, and wrong-Codex refusal fixtures; explicit
  missing, changed, extra-regular-file, symlink, and hard-link tampering; `git
  diff --check`; and a no-index whitespace check for the untracked collector.
- Verification result: every syntax, repetition, deterministic collection,
  manifest, refusal, no-mutation, tamper, and diff-hygiene check passed. All 50
  wrong-Codex repetitions returned the intended status 64 with unchanged
  authority evidence.
- What remains: `EXT-18`, the reproducible versioned Level-1 release
  candidate. Blockers for EXT-17: none. Real dogfood remains intentionally
  uncollected until exact candidate and remote-CI authority exist;
  external-project use remains unapproved.

## Controller Rejection — EXT-17 Flaky Wrong-Codex Refusal Oracle (2026-07-16)

- EXT-17 is not complete. In the controller's independent complete matrix, the
  wrong-Codex fixture exited `1` with `rejected wrong-codex input changed Git
  authority` instead of proving the required pre-mutation refusal.
- Five immediate isolated repetitions of the same wrong-Codex case exited the
  intended `64` with unchanged-authority evidence. That inconsistency makes
  the refusal/no-mutation oracle nondeterministic; a passing retry does not
  erase the failed required verification occurrence.
- The hard-link repair itself passed: both original bundles verified, missing,
  changed, extra-file, symlink, and hard-link tampering were rejected, and the
  reproduced aliased files had link count two. Syntax, canonical-manifest
  comparison, the other refusal cases, and diff hygiene also passed.
- Repair requires reproducing and removing the wrong-Codex Git-authority
  nondeterminism, retaining enough diagnostic evidence to identify any future
  mismatch, and rerunning the complete EXT-17 matrix reliably.
- EXT-17 is restored to unchecked. EXT-18 has not been assessed or authorized.
  Blocker: none; run one fresh pass on EXT-17 only.

## EXT-17 Hard-Link Alias Repair And Fresh Verification (2026-07-16)

- Task selected: `EXT-17`, repair and freshly verify the opt-in Level-1
  dogfood evidence collector after the reproduced hard-link substitution.
- Inventory creation and manifest verification now require every regular
  evidence file to have exactly one link before and after hashing. The
  manifest, file inventory, and bundle digest receive the same single-link
  check, so byte-identical paths cannot share inode authority.
- Fixture-only collection permanently copies its completed bundle, replaces
  `identity/doctor.err` with a hard link to the byte-identical
  `identity/config-check.err`, and requires verification of that copied bundle
  to fail without changing the retained fixture evidence.
- Files changed in this fresh pass:
  `scripts/dogfood-external-level1.sh`, `.agent/TASKS.md`, and this state file.
  No dependency, production Go code, durable architecture decision, or commit
  was added.
- Verification commands: `bash -n scripts/dogfood-external-level1.sh`; two
  independent `--fixture-only` collections; canonical manifest comparison
  after removing only `collected_at_utc`; `--verify-manifest` for both original
  bundles; all four dirty/non-disposable/wrong-binary/wrong-Codex refusal
  fixtures; explicit missing, changed, extra-regular-file, symlink, and
  reproduced hard-link tampering; `git diff --check`; and a no-index whitespace
  check for the untracked collector.
- Verification result: every required collection, determinism, validation,
  refusal, tamper, and diff-hygiene check passed. The hard-linked
  `config-check.err`/`doctor.err` pair had link count two and was rejected.
- What remains: EXT-18, the reproducible versioned Level-1 release candidate.
  Blockers for EXT-17: none. Real dogfood remains intentionally uncollected
  until exact candidate and remote-CI authority exist; external-project use
  remains unapproved.

## Controller Rejection — EXT-17 Manifest Alias Verification (2026-07-16)

- EXT-17 is not complete. Its durable decision says bundle verification
  rejects aliased evidence, but independent verification replaced
  `identity/doctor.err` with a hard link to the byte-identical
  `identity/config-check.err`; both paths then had link count two and
  `--verify-manifest` still exited zero.
- The required syntax check, two independent fixture-only collections,
  canonical-manifest comparison, verification of both original bundles, all
  four pre-admission refusal fixtures, `git diff --check`, and no-index script
  whitespace check passed. Missing, changed, extra-regular-file, and symlink
  tampering were also correctly rejected.
- Repair requires inventory creation and verification to reject aliased
  regular evidence, with a permanent regression for the reproduced hard-link
  substitution, followed by the complete EXT-17 verification matrix.
- EXT-17 is restored to unchecked. EXT-18 has not been assessed or authorized.
  Blocker: none; run one fresh pass on EXT-17 only.

## EXT-17 Level-1 Dogfood Evidence Collector (2026-07-16)

- Task selected: `EXT-17`, add the opt-in Level-1 external-project dogfood
  evidence collector.
- `scripts/dogfood-external-level1.sh` now refuses a real operation unless the
  external repository is clean, non-bare, explicitly and doubly identified as
  disposable, and bound to the exact approved config, candidate binary hash,
  clean Go VCS source revision, Revolvr version output, listed Codex version/
  digest/path, task, operation, finite cycle bound, expected typed outcome,
  outside sentinel, declared UTC evidence time, and new external evidence
  directory. Candidate config check and exact-task attended doctor must pass
  before the evidence directory or operation is started.
- Each operation bundle records before/after source HEAD, branch, status,
  index, refs, diffs, worktrees, canonical task/runtime trees, and complete
  outside-sentinel metadata/content; effective config and resource bounds;
  task/state/operation history, runs, receipts, completion, workspace, ledger,
  export/replay validation, resource/disk/output use, and the typed outcome.
  The manifest and every regular evidence file are covered by a canonical
  SHA-256 inventory plus an inventory digest, and `--verify-manifest` rejects
  missing, extra, symlinked, or changed evidence.
- `--fixture-only` builds a deterministic VCS-stamped candidate and disposable
  external fixture without invoking a model. Two independent bundles produced
  identical manifests after removing only the declared `collected_at_utc` row
  and both verified. Fixture faults for dirty, non-disposable, wrong-candidate,
  and wrong-Codex input each exited nonzero after proving the complete source
  tree, semantic Git/index authority, and outside sentinel unchanged and that
  no evidence directory was created.
- Files changed for EXT-17: `scripts/dogfood-external-level1.sh`,
  `.agent/TASKS.md`, `.agent/DECISIONS.md`, and this state file. No dependency
  or commit was added, and no real Codex process was started.
- Verification commands: `bash -n scripts/dogfood-external-level1.sh`; two
  executions of `scripts/dogfood-external-level1.sh --fixture-only`; canonical
  manifest comparison after removing only the declared collection-time row;
  `--verify-manifest` for both bundles; all four refusal fixtures with absence
  of evidence assertions; `git diff --check`; and a no-index whitespace check
  for the new untracked script.
- Verification result: the complete required matrix passed after one repair.
  The first full refusal matrix exposed that a read-only Git status can refresh
  volatile `.git` timestamps; the repaired no-mutation oracle excludes those
  timestamps while separately comparing exact source evidence, refs, status,
  diffs, worktree registrations, and raw index bytes/mode/size/link count.
- What remains: EXT-18, the reproducible versioned Level-1 release candidate.
  Blockers for EXT-17: none. Real dogfood evidence remains intentionally
  uncollected until the exact EXT-18/EXT-19 candidate authority exists, and
  external-project use remains unapproved.

## EXT-16 Attended External-Project Runbook (2026-07-16)

- Task selected: `EXT-16`, write and smoke-test the attended external-project
  operator runbook.
- `docs/external-project-runbook.md` covers immutable release/hash pinning,
  initialization and protected path modes, attended configuration and safety
  responsibilities, every finite Level-1 default and its preflight/run
  evidence, task authoring/import/migration/scheduling/checkpoint/input,
  foreground start/monitor/cancel/restart, evidence inspection, every typed
  Level-1 stop and recovery boundary, exact confirmed reconciliation,
  workspace review and operator-only integration/removal, archive, export,
  retention, upgrade, and guarded runtime-state retirement. Queue and daemon
  remain explicitly unapproved.
- `scripts/smoke-external-attended.sh` builds a disposable versioned binary
  and external Git repository, exercises every documented non-destructive
  command or safe refusal plus all referenced help surfaces, verifies ledger
  export/replay and retention planning, proves no Codex execution, and safely
  retires only the disposable runtime tree.
- Notification list/show now resolve an omitted work directory through the
  shared app state-path boundary. Focused CLI coverage proves current-directory
  resolution and that missing notification reads create no runtime state.
- Files changed for EXT-16: `docs/external-project-runbook.md`,
  `scripts/smoke-external-attended.sh`, `internal/app/notification_inspect.go`,
  `internal/cli/autonomous_run_test.go`, `.agent/TASKS.md`, and this state
  file. No dependency or commit was added; no durable architecture decision
  changed.
- Verification commands: `gofmt -w internal/app/notification_inspect.go
  internal/cli/autonomous_run_test.go`; `go test -count=1 ./internal/cli -run
  '^TestNotificationWarningRenderingAndReadOnlyInspection$'`; `bash -n
  scripts/smoke-external-attended.sh`; `bash
  scripts/smoke-external-attended.sh`; `go test -count=1 ./...`; `go run
  ./cmd/revolvr --help` and 34 referenced subcommand help invocations; and
  `git diff --check`.
- Verification result: the focused test, shell syntax check, complete
  disposable smoke, full Go suite, help inventory, formatting, and whitespace
  checks passed. The first complete smoke after the notification repair found
  fixture cleanup paths anchored to the source checkout; the one permitted
  repair computed the guarded retirement paths inside the disposable fixture,
  and the rerun passed.
- What remains: EXT-17, the opt-in Level-1 dogfood evidence collector. There
  are no blockers for EXT-16; external-project use remains unapproved until
  the remaining ordered release gates pass.
- Blockers: none.

## EXT-16 Attended Runbook Blocked (2026-07-16)

- Task selected: `EXT-16`, write and smoke-test the attended external-project
  operator runbook.
- `docs/external-project-runbook.md` now covers release/hash pinning,
  initialization and protected path modes, attended configuration and safety
  responsibilities, every finite Level-1 default and its preflight/run
  evidence, task authoring/import/migration/scheduling/checkpoint/input,
  foreground start/monitor/cancel/restart, evidence inspection, typed stops,
  every Level-1 recovery boundary, explicit task recovery, workspace review
  and operator-only integration/removal, archive, export/retention, upgrade,
  and guarded runtime-state retirement. It explicitly marks queue and daemon
  unapproved.
- `scripts/smoke-external-attended.sh` builds a disposable versioned binary,
  creates and initializes an external Git fixture, uses a no-model unlisted
  fake Codex, exercises the documented non-destructive/read-only commands and
  safe refusals, checks referenced command help, and guards runtime retirement.
- Files changed in this pass: `docs/external-project-runbook.md`,
  `scripts/smoke-external-attended.sh`, and this state file. No dependency or
  commit was added. `.agent/TASKS.md` remains unchanged and EXT-16 remains
  unchecked.
- Verification commands run: `bash -n
  scripts/smoke-external-attended.sh`; `git diff --check --
  docs/external-project-runbook.md scripts/smoke-external-attended.sh`; and
  `bash scripts/smoke-external-attended.sh` twice. The first smoke run exposed
  an invalid empty-body Markdown import fixture; the one permitted repair added
  explicit task-body text. Shell syntax and the initial diff check passed.
- Verification result: blocked after the repaired full smoke run reached the
  production command `revolvr notification list`. In an ordinary initialized
  current-directory fixture it returns `harness runtime path: repository root
  is required`. `internal/cli` passes an empty default `Options.WorkDir` to
  `app.ListNotifications`, which forwards it directly to
  `autonomousnotification.List` instead of resolving the current canonical
  repository root as other app read projections do.
- What remains: in a fresh EXT-16 pass, make notification list/show resolve
  the omitted work directory through the ordinary app state-path boundary,
  add focused CLI coverage, then rerun the complete required smoke/help/diff
  verification. Re-review the generated runbook/script diff before marking
  EXT-16 complete.
- Blocker: the production notification inspection root-resolution defect
  prevents the required runbook smoke from completing. No second repair was
  attempted in this pass, as required by the fresh-loop verification rule.

## EXT-15 Exact-Candidate Release CI Matrix (2026-07-16)

- Task selected: `EXT-15`, make the complete release CI matrix mandatory for
  the exact candidate commit.
- `.github/workflows/ci.yml` now triggers on every pushed branch and tag plus
  every pull request, with no path filters, job conditions, or dependency
  chains. Independent required jobs cover the Go 1.22 source floor and full
  suite, the six-test production autonomous strict-fake suite, the full race
  suite, `go vet`, module verification, each fake-Codex smoke path, supported
  Linux/macOS/FreeBSD amd64 builds, and the separate Windows diagnostic stub.
  Every job compares checked-out `HEAD` with `GITHUB_SHA` and publishes that
  exact source commit in the job summary.
- The two mixed-pass smoke fakes now report the structurally exact version
  `codex-cli 1.2.3`. That unlisted identity remains diagnostic rather than
  release authority, while the smoke fixtures can exercise the current config
  and run-once paths instead of failing on the obsolete `fake-codex` version
  grammar.
- Files changed for EXT-15: `.github/workflows/ci.yml`,
  `scripts/smoke-run-once-fake-codex.sh`,
  `scripts/smoke-run-once-fake-codex-verification-failure.sh`,
  `.agent/TASKS.md`, `.agent/DECISIONS.md`, and this state file. No dependency
  or commit was added.
- Verification commands: a PyYAML parse plus explicit trigger/job/command/SHA
  structural assertions; `GOTOOLCHAIN=go1.22.12 go version`; Go 1.22.12
  `go test ./...`; the explicit six-test production strict-fake command; full
  host and Go 1.22.12 `go test -race -count=1 ./...`; Go 1.22.12 `go mod
  verify` and `go vet ./...`; Bash syntax and execution of both fake-Codex
  smokes; Go 1.22.12 Linux, Darwin, FreeBSD, and Windows amd64 builds; the
  Windows unsupported-platform string assertion; and `git diff --check`.
- Verification result: workflow syntax/structure and every locally
  reproducible job passed. The first smoke execution exposed both stale fake
  version strings; the single bounded repair changed the reported and expected
  versions to the current exact grammar, after which both host and Go 1.22.12
  smoke executions passed. Remote execution proof remains intentionally owned
  by EXT-19.
- What remains: EXT-16 and the later ordered external-readiness tasks.
  Blockers for EXT-15: none. Autonomous external-project use remains
  unapproved.

## EXT-14 Fresh Verification Pass (2026-07-16)

- Task selected: `EXT-14`, prove Level-1 task and explicit-administration
  interruption recovery at every production durable transition seam.
- The existing production matrix and nil-by-default failure hooks were
  inspected for all 18 before/after task, notification, and archive seams.
  Task restart preserves the stable operation and stops in-flight work as
  `unsafe_or_ambiguous`; notification replay retains its delivery ID; archive
  restart rolls the admitted journal forward without duplicating publication.
- Files changed in this fresh pass:
  `internal/app/production_interruption_recovery_test.go`, `.agent/TASKS.md`,
  and this state file. Receipt evidence now compares the exact artifact path,
  mode, and content-hash tree across restart rather than only its count. No
  dependency or commit was added.
  `.agent/DECISIONS.md` was not changed because the durable interruption
  ownership decision was already recorded.
- Verification commands: `gofmt -w` on the seven EXT-14 Go files; `go test
  -count=1 ./internal/app -run
  '^TestProductionTaskInterruptionRecoveryMatrix$'`; the same focused command
  with `-race`; `go test -count=1 ./...`; a final `gofmt -l` check on the seven
  EXT-14 Go files; and `git diff --check`.
- Verification result: the ordinary 18-seam matrix, race-enabled matrix, and
  complete repository suite passed. Stable task operation/delivery/archive
  identities replayed without duplicate commits, attempt charges,
  notification success claims, task completion, terminal ledger evidence,
  receipts, completion artifacts, or archives. The first focused rerun exposed
  that a numeric at-most-one receipt assertion rejected two legitimate worker
  receipts at later seams; the single repair replaced it with exact receipt-
  tree equality, which directly proves restart adds or rewrites no receipt.
  Formatting and diff-hygiene checks also passed.
- What remains: EXT-15 and the later ordered external-readiness tasks.
  Blockers for EXT-14: none. Autonomous external-project use remains
  unapproved until the remaining release gates pass.

## EXT-13 Read-only Recovery Repair (2026-07-16)

- Task selected: `EXT-13`, repair the Level-1 autonomous recovery command so
  default inspection remains strictly read-only when the live workspace HEAD
  has drifted from durable authority.
- `autonomousworkspace.Inspect` now verifies the same marker, deterministic
  workspace identity, Git common directory, worktree registration, linked
  `.git` file, branch, HEAD, tree, source revision, and cleanliness as reopen,
  but acquires no Git-administration mutation lease and never publishes a
  retained ambiguity ref. The mutating reopen path retains its established
  retained-ref recovery behavior.
- App recovery uses that read-only projection and reports expected and
  observed Git HEAD/tree/source identities. The focused regression advances
  the real task-workspace HEAD, proves the Git authority check fails, and
  compares every Git ref plus the complete repository contents and metadata
  before and after inspection.
- Files changed for this pass: `internal/autonomousworkspace/manager.go`,
  `internal/app/autonomous_recovery.go`,
  `internal/app/autonomous_recovery_test.go`, `.agent/TASKS.md`,
  `.agent/DECISIONS.md`, and this state file.
- Verification commands: `gofmt -w` on the three changed Go files; `go test
  -count=1 ./internal/app ./internal/cli -run
  'Test(RecoverAutonomousTaskRequiresExactReconciliation|TaskRecoveryCommand)$'`;
  the same focused command with `-race`; `go run ./cmd/revolvr task --help`;
  and `go test -count=1 ./...`.
- Verification result: the focused ordinary/race tests, CLI help, and complete
  repository suite passed. The advanced-HEAD case retained byte-for-byte and
  metadata-for-metadata repository evidence and the exact complete ref set.
- What remains: EXT-14 and the later ordered external-readiness tasks.
  Blockers for EXT-13: none. External use remains unapproved.

## Controller Rejection — EXT-13 Read-only Recovery (2026-07-16)

- EXT-13 is not complete. `inspectAutonomousRecovery` calls
  `autonomousworkspace.Reopen`; that path acquires the Git administration lock
  and calls `git update-ref` to publish a retained ambiguity ref when the live
  task-workspace HEAD differs from durable authority. Therefore the default
  `revolvr task recover` inspection is not read-only for the required drift
  case.
- The existing read-only test snapshots only an agreeing workspace and does
  not exercise advanced-HEAD/ref drift, so its passing result does not satisfy
  the exact acceptance criterion.
- Repair requires a genuinely non-mutating workspace/Git inspection path and
  a regression proving complete refs and runtime/source evidence remain
  unchanged when live workspace authority has drifted.
- EXT-14 was completed in the same prior pass and is restored to unchecked so
  it receives its own fresh invocation after EXT-13 passes.
- Blocker: none; run one fresh pass on EXT-13 only.

## EXT-14 Production Interruption Recovery Matrix (2026-07-16)

- Task selected: `EXT-14`, prove Level-1 task and explicit-administration
  interruption recovery at every production durable transition seam.
- Nil-by-default failure injection now brackets supervisor, worker,
  verification, commit, checkpoint, audit, finalization, notification
  delivery, and archive manifest publication. The production matrix exercises
  all 18 before/after points with stable operation or delivery identities.
- Interrupted task operations retain their durable in-flight authority and
  restart as `unsafe_or_ambiguous` without rerunning Codex or changing the
  already-published domain effects. Notification restart reuses its stable
  delivery ID, and archive restart rolls the same admitted journal forward.
- Archive recovery now reconstructs and verifies the exact journal-bound
  manifest when interruption occurred before immutable manifest publication;
  a manifest already published before journal advancement remains the restart
  authority.
- The matrix proves no duplicate source commit, attempt admission/completion,
  notification success, completed task, terminal ledger event, receipt,
  completion artifact, archive entry, or administrative archive commit.
- Files changed for EXT-14: `internal/autonomouscycle/types.go`,
  `internal/autonomouscycle/cycle.go`, `internal/autonomouscycle/worker.go`,
  `internal/app/autonomous_run.go`, `internal/app/notification.go`,
  `internal/autonomousarchive/coordinator.go`,
  `internal/app/production_interruption_recovery_test.go`, `.agent/TASKS.md`,
  `.agent/DECISIONS.md`, and this state file.
- Verification commands: `gofmt -w` on all changed Go files;
  `go test -count=1 ./internal/app -run
  '^TestProductionTaskInterruptionRecoveryMatrix$'`; the same focused command
  with `-race`; `go test -count=1 ./...`; `git diff --check`; and `gofmt -l`
  on the changed Go files.
- Verification result: the focused 18-seam matrix, focused race run, and full
  repository suite passed. Formatting and whitespace checks passed.
- What remains: EXT-15 and the later ordered external-readiness tasks. There
  are no blockers for EXT-14; autonomous external-project use remains
  unapproved until the remaining release gates pass.

## EXT-13 Explicit Operator Recovery (2026-07-16)

- Task selected: `EXT-13`, add the Level-1 read-only autonomous task recovery
  inspection and exact confirmed reconciliation command.
- `revolvr task recover <task-id> --operation-id <id>` now reports ordered
  task, state, workspace, Git, ledger, receipt, and artifact authority without
  starting a model or mutating recovery state. Missing artifact authorities are
  reported explicitly when no completed run exists.
- Reconciliation requires both `--reconcile` and
  `--confirm-operation <operation-id>`, applies only to terminal
  `unsafe_or_ambiguous` operations, repeats all authority checks under the
  execution lease, refuses drift, and publishes a deterministic new admitted
  operation linked to the immutable old operation and authority digest.
- Existing retry and unblock commands remain independent and cannot invoke or
  clear autonomous recovery. Focused application and CLI tests prove the
  read-only tree, exact confirmation, immutable old operation, deterministic
  replay, and refusal after task drift.
- One verification repair moved the fully-unattended daemon mode prerequisite
  ahead of ambient Codex identity admission. Invalid operator-attended daemon
  requests now fail for their requested mode, while valid unattended requests
  still perform the complete external identity admission.
- Files changed for EXT-13: `internal/app/autonomous_recovery.go`,
  `internal/app/autonomous_recovery_test.go`, `internal/app/autonomous_run.go`,
  `internal/autonomoustaskrun/recovery.go`,
  `internal/autonomoustaskrun/ledger.go`, `internal/cli/root.go`,
  `internal/cli/task_recovery_test.go`, `.agent/TASKS.md`,
  `.agent/DECISIONS.md`, and this state file.
- Verification commands: `gofmt -w` on all changed Go files;
  `go test -count=1 ./internal/app ./internal/cli -run
  'Test(RecoverAutonomousTaskRequiresExactReconciliation|TaskRecoveryCommand)$'`;
  the same focused command with `-race`;
  `go test -count=1 ./internal/autonomoustaskrun`;
  `go run ./cmd/revolvr task --help`; `go test -count=1 ./...`;
  `git diff --check`; and `gofmt -l` on the changed Go files.
- Verification result: all required focused, race, CLI, package, full-suite,
  formatting, and whitespace checks passed after the single repair attempt.
- What remains: EXT-14 and the later ordered external-readiness tasks. There
  are no blockers for EXT-13; autonomous external-project use remains
  unapproved until the remaining release gates pass.

## EXT-12 External Interruption And Recovery Contract (2026-07-16)

- Task selected: `EXT-12`, publish the settled interruption and recovery
  contract as a complete transition-seam matrix.
- `docs/external-recovery.md` defines the shared exact-replay boundary, fresh-
  ephemeral-process rule, immutable old-operation authority, task-scoped
  `unsafe_or_ambiguous` quarantine, Level-1 stop behavior, Level-2/3 unrelated-
  work continuation only after durable exclusion, and the prohibition on
  generic retry or manual runtime-state edits clearing quarantine.
- Ten three-row tables cover before, during, and after supervisor, worker,
  verification, commit, checkpoint, audit, finalization, queue reconciliation,
  notification, and archive publication. Every row explicitly names durable
  restart authority, exact replay, ambiguity handling, permitted L1/L2/L3
  continuation, prohibited inference, and the exact operator inspection or
  reconciliation action.
- The contract distinguishes at-least-once notification recovery from task
  quarantine, preserves receiver-side stable-key deduplication, keeps archive
  administration explicit, and requires the future `task recover` command to
  preserve the old operation while creating a new identity only after exact
  reconciliation.
- Files changed for EXT-12: `docs/external-recovery.md`, `.agent/TASKS.md`, and
  this state file. `.agent/DECISIONS.md` was not changed because the document
  applies the already-settled readiness and owner contracts without adding an
  implementation or architecture decision.
- Verification commands: the required `rg -n` term scan from EXT-12;
  heading/row-count scans proving all ten seams and exactly 30 timing rows;
  `git diff --check`; a no-index `git diff --check` of the new untracked
  document (no whitespace diagnostics; the expected content-difference status
  was 1); and a manual row-by-row cross-check against
  `.agent/AUTONOMOUS_EXTERNAL_READINESS.md` and `.agent/DECISIONS.md` covering
  in-flight quarantine, immutable history/checkpoint precedence, process
  settlement, attempt/verification/commit uniqueness, finalization roll-
  forward, queue exclusion/order, notification idempotency, and archive
  reconciliation.
- Verification result: all required terms, ten seam headings, and 30 matrix
  rows are present; diff hygiene passed; every row agrees with the settled
  readiness and durable owner decisions. No dependency, production code, or
  commit was added.
- What remains: EXT-13 and the later ordered external-readiness gates. Blockers
  for EXT-12: none. External use remains unapproved.

## EXT-11 External Git Containment Edge Matrix (2026-07-16)

- Task selected: `EXT-11`, the real-Git dirty, staged, ignored,
  linked-worktree, SHA-1, SHA-256, concurrent external-commit, and active-
  submodule containment matrix.
- `TestExternalGitContainmentMatrix` enters the public attended admission path
  for dirty, staged, and active-submodule refusals and proves no task step or
  ref/workspace publication begins. The ignored case passes clean Git
  admission, then proves the app workspace preparation boundary rejects
  policy-relevant ignored source before creating the task ref or linked
  workspace.
- Positive real-Git cases create exact task workspaces and path-scoped commits
  from SHA-1 and SHA-256 repositories, including a control root that is itself
  a linked worktree. They prove exact object-ID length, task branch/ref,
  baseline parent, one-path commit delta and bytes, clean matching index/tree,
  workspace reconciliation, and unchanged operator/unrelated authority.
- The concurrent case injects a real operator commit on the control branch
  after the task commit's pre-HEAD lookup and before task staging. The operator
  and task commits remain on their exact independent branches and neither tree
  absorbs the other's path.
- Outside and unrelated-worktree sentinels contain regular, executable,
  symlink, and hard-linked entries. Complete snapshots prove exact entries,
  bytes, modes, targets, timestamps, and link counts plus unrelated branch,
  HEAD, status, and index authority. Index authority itself compares exact
  bytes, size, type, mode, and link count while excluding the non-semantic file
  timestamp that ordinary read-only Git status refreshes.
- Files changed for EXT-11:
  `internal/app/external_git_containment_test.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w
  internal/app/external_git_containment_test.go`; `go test -count=1
  ./internal/app -run '^TestExternalGitContainmentMatrix$'`; `go test -race
  -count=1 ./internal/app -run '^TestExternalGitContainmentMatrix$'`; `go test
  -count=1 ./...`; a final `gofmt -l` check; and `git diff --check`.
- Verification result: the required focused ordinary/race runs and the full
  repository suite passed. The first focused run refined the fixture oracle to
  distinguish exact index content authority from Git's timestamp-only status
  refresh and normalized linked-checkout `.agent` permissions under the host
  umask; the complete final matrix then passed. No production code, dependency,
  or commit was added.
- What remains: EXT-12 and the later ordered external-readiness gates. Blockers
  for EXT-11: none. External use remains unapproved.

## EXT-10 Run-Owned Commit And Git-Operation Containment (2026-07-16)

- Task selected: `EXT-10`, exact run-owned commit containment plus the
  prohibited production Git-operation and unrelated-worktree boundary.
- The shared commit gate now applies `--literal-pathspecs` and `--only` to the
  exact same sorted paths admitted for staging. A real-Git regression captures
  one source change plus required task metadata, injects unrelated staged,
  tracked-worktree, and untracked bytes afterward, and proves the exact commit
  delta/tree while every late byte and the unrelated staged index entry remain
  unchanged.
- The production happy-path fixture now has a command-spy form that executes
  the ordinary runner, fails immediately on push, merge, rebase, reset, clean,
  or stash, observes real workspace and commit operations, and proves a second
  linked worktree retains its branch, HEAD, status, tracked bytes, untracked
  sentinel bytes, and sentinel mode. The unused destructive workspace restore
  function and its reset/clean-only support code were removed; no production
  caller existed.
- Files changed for EXT-10: `internal/commit/commit.go`,
  `internal/commit/commit_test.go`,
  `internal/app/production_autonomous_happy_path_test.go`,
  `internal/autonomousworkspace/manager.go`,
  `internal/autonomousworkspace/manager_test.go`,
  `internal/runonce/runonce_test.go`, `.agent/TASKS.md`, this state file, and
  `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on every changed Go file; the required
  focused ordinary and race commands for
  `TestExternalCommitContainsOnlyRunOwnedDelta|TestProductionAutonomyForbidsRepositoryIntegrationOps`;
  complete commit, autonomous-workspace, and app package tests; `go test
  -count=1 ./...`; a production prohibited-Git-verb source scan; `gofmt -l` on
  every changed Go file; and `git diff --check`.
- Verification result: the required focused ordinary/race runs, all directly
  affected package tests, and the final full repository suite passed. The
  first full-suite run exposed one legacy runonce assertion for the former
  unscoped commit argv; the single expectation repair records the new exact
  literal path-scoped command, and the final full suite passed. No dependency
  or commit was added.
- What remains: EXT-11 and the later ordered external-readiness gates.
  Blockers for EXT-10: none. External use remains unapproved.

## EXT-09 Exact Task-Workspace Authority (2026-07-16)

- Task selected: `EXT-09`, exact task-scoped branch, linked-workspace,
  baseline, control-root, Git-common-directory, registration, marker, and
  current-HEAD authority for external autonomous work.
- `TestExternalTaskWorkspaceAuthority` proves a task workspace is created from
  the requested exact baseline on its deterministic `refs/heads/revolvr/tasks/`
  branch while the ambient operator branch and its newer bytes remain
  untouched. The complete control/execution roots, Git common directory,
  branch, baseline, HEAD, checkpoint, and ownership-marker identities survive
  deterministic durable JSON projection.
- Reopen and commit reconciliation now compare every deterministic workspace
  identity, including the ownership-marker path, before acquiring a Git-admin
  lock. Reusing the task execution worktree as a changed control root therefore
  fails before it can create `.revolvr` state inside task source.
- Ownership markers use the descriptor-rooted runtime-path boundary for
  creation, synchronization, and reads. Linked-worktree `.git` files use a
  bounded no-follow open with regular-file/single-link and pre/post identity
  checks. Marker and `.git` symlinks, foreign workspace paths, common
  directories, refs, baselines, and registrations all fail while exact source
  HEAD, branch, status, and tracked bytes remain unchanged.
- Files changed for EXT-09: `internal/autonomousworkspace/manager.go`,
  `internal/autonomousworkspace/manager_test.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on both changed Go files; `go test
  -count=1 ./internal/autonomousworkspace -run
  '^TestExternalTaskWorkspaceAuthority$'`; the same focused command with
  `-race`; `go test -count=1 ./internal/autonomousworkspace`; `go test
  -count=1 ./...`; Darwin and FreeBSD amd64 package test cross-compiles;
  `gofmt -l` on both changed Go files; and `git diff --check`.
- Verification result: the required focused ordinary/race runs, complete owner
  package, full repository suite, both supported-platform cross-compiles,
  formatting, and diff checks passed. The first focused run exposed that Git
  may create linked-worktree parent directories with the invoking umask; the
  repair kept marker storage on the strict runtime boundary while validating
  the Git-created `.git` file directly with no-follow identity checks. No
  dependency or commit was added.
- What remains: EXT-10 and the later ordered external-readiness gates. Blockers
  for EXT-09: none. External use remains unapproved.

## EXT-08 Production Attended Terminal Matrix (2026-07-16)

- Task selected: `EXT-08`, the production attended-task terminal-outcome
  matrix through `app.RunTaskUntilTerminal` with no injected task runner.
- One table-driven strict-fake production test now proves separate
  `needs_input`, authorized `blocked`, verification-failure
  `unsafe_or_ambiguous`, identical-strategy `no_progress`, trusted
  `safety_stop`, caller `operation_cancelled`, exact terminal authority replay,
  and `max_cycles` outcomes. Every case enters the public app boundary and the
  ordinary production workspace, supervisor, worker, attempt, verification,
  commit, state, task-run, and ledger composition.
- The matrix asserts exact stop detail and cycle/replay facts; strict model
  invocation/version counts; canonical task bytes/status; control and
  workspace HEAD/status/commit authority; workspace checkpoint identity;
  receipt and verification presence/absence; exact allowed state-history
  categories; task-run ledger shape; model/verification/commit event counts;
  and the absence of completion artifacts, finalization state/runs/events,
  unrelated state history, or other unauthorized effects.
- Trusted supervisor read-only mutation is now preserved as `safety_stop`
  using the cycle's typed `SourceDifference.Changed` evidence. Other
  supervisor failures and verification failures remain
  `unsafe_or_ambiguous`; error prose does not grant trusted safety authority.
- Files changed for EXT-08:
  `internal/app/production_autonomous_terminal_test.go`,
  `internal/app/autonomous_run.go`, `.agent/TASKS.md`, this state file, and
  `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w internal/app/autonomous_run.go
  internal/app/production_autonomous_terminal_test.go`; `go test -count=1
  ./internal/app -run '^TestProductionAutonomousTerminalMatrix$'`; `go test
  -race -count=1 ./internal/app -run
  '^TestProductionAutonomousTerminalMatrix$'`; `go test -count=1 ./...`;
  `git diff --check`; and a final `gofmt -l` check on both changed Go files.
- Verification result: every required focused, race, and full repository test
  passed; formatting and diff checks passed. The first focused run corrected
  exact porcelain-status expectations, nil/empty receipt comparison, the
  two-cycle no-progress fixture, and cancellation contract setup. The tightened
  final matrix then passed ordinary and race execution. No dependency or commit
  was added.
- What remains: EXT-09 and the later ordered external-readiness gates.
  Blockers for EXT-08: none. External use remains unapproved.

## EXT-07 Production Correction And Re-audit (2026-07-16)

- Task selected: `EXT-07`, production correction, distinct final verification,
  exact finding resolution, and clean independent re-audit through
  `app.RunTaskUntilTerminal` without an injected task runner.
- The strict-fake operation records one blocking `incorrect-result` finding,
  admits exactly one correction attempt, changes and commits only
  `docs/result.md`, runs a distinct final verification occurrence, resolves
  the exact finding, runs a distinct clean auditor process, advances one
  checkpoint, and completes. The test asserts the exact attempt pairs, single
  source commit and diff, two verification occurrences, two audit runs, three
  worker receipts, audit-history ordering, frozen evidence, ledger runs, and
  separate control/workspace source authority.
- Top-level audit application now validates its dossier against the exact
  pre-admission execution state while accepting the current state only when it
  is a legal successor differing solely by append-only attempt accounting.
  Verification provenance is compared through its canonical JSON projection,
  matching the worker-visible schema while still requiring exact persisted
  values.
- Correction re-audit receives an ephemeral workspace/state projection at the
  exact corrected commit and source revision; durable checkpoint authority is
  still advanced only after correction, final verification, resolution, and
  clean audit all succeed. Reopening stored audit evidence accepts the same
  exact-or-trimmed profile identity contract used at initial audit admission.
- Files changed for EXT-07:
  `internal/app/production_autonomous_correction_test.go`,
  `internal/app/autonomous_run.go`,
  `internal/autonomousauditapply/apply.go`,
  `internal/autonomouscorrection/coordinator.go`,
  `internal/autonomousstate/audit_store.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on changed Go files; `go test -count=1
  ./internal/app -run '^TestProductionAutonomousCorrectionAndReaudit$'`;
  `go test -count=1 -race ./internal/app -run
  '^TestProductionAutonomousCorrectionAndReaudit$'`; `go test -count=1
  ./internal/autonomousauditapply ./internal/autonomousstate
  ./internal/autonomouscorrection`; `go test -count=1 ./...`; and `git diff
  --check`.
- Verification result: every focused, race, directly affected package, and
  full repository test passed; formatting and diff checks passed. No
  dependency or commit was added.
- What remains: EXT-08 and the later ordered external-readiness gates.
  Blockers for EXT-07: none. External use remains unapproved.

## EXT-06 Production-Composition Happy Path (2026-07-16)

- Task selected: `EXT-06`, the complete attended autonomous happy path through
  `app.RunTaskUntilTerminal` with no injected `TaskRunInput.Runner`.
- One strict-fake operation now reaches the real `productionStepRunner` and
  proves exact workspace creation, supervisor document decision, documentor
  action, attempt admission/completion, final tier verification, run-owned
  commit, checkpoint advancement, fresh independent audit, complete decision,
  frozen evidence, canonical task/state terminalization, and finalization/task
  ledger completion. It asserts exact source placement, Git diff and heads,
  receipt bytes, state/task bytes, workspace marker material identity,
  completion evidence/manifest identities, run/event sets, and task-run
  operation bytes.
- The production path now propagates exact admitted Codex/Git identities and a
  release manifest to every supervisor/worker invocation, injects deterministic
  IDs only through unexported test seams, rehydrates current mutation,
  verification, and audit authority from durable audit history between cycles,
  and carries the worker's role-projected dossier as worker evidence.
- Optional-role composition gives its mandatory nested audit the exact
  post-commit ephemeral workspace authority before the durable checkpoint is
  advanced. Audit application admits the exact auditor role dossier while
  independently retaining supervisor-dossier provenance, and accepts either
  exact profile-file identity or the prompt loader's whitespace-normalized
  profile identity. Checkpoint advancement treats the just-created verified
  run-owned commit as trusted input, while terminal revalidation recognizes
  only its exact matching in-progress finalization envelope.
- Files changed for EXT-06: `internal/app/production_autonomous_happy_path_test.go`,
  `internal/app/autonomous_run.go`, `internal/app/external_admission.go`,
  `internal/app/strict_fake_codex_test.go`,
  `internal/app/testdata/strictfakecodex/main.go`,
  `internal/codexexec/codexexec.go`, `internal/supervisor/execution.go`,
  `internal/autonomouscycle/types.go`, `internal/autonomouscycle/cycle.go`,
  `internal/autonomouscycle/worker.go`,
  `internal/autonomousoptional/coordinator.go`,
  `internal/autonomousauditapply/apply.go`, `.agent/TASKS.md`, this state file,
  and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on changed Go files; `go test -count=1
  ./internal/app -run TestProductionAutonomousHappyPath`; `go test -race
  -count=1 ./internal/app -run TestProductionAutonomousHappyPath`; `go test
  -count=1 ./internal/autonomousauditapply`; `go test -count=1 ./...`; and
  `git diff --check`.
- Verification result: every required focused, race, compatibility-package,
  and full repository test passed. The first full-suite run exposed legacy
  audit fixtures that retained exact profile-file bytes rather than the prompt
  loader's normalized bytes; the single compatibility repair accepts either
  exact representation while preserving the expected digest and size, and the
  final full run passed. No dependency or commit was added.
- What remains: EXT-07 and the later ordered external-readiness gates.
  Blockers for EXT-06: none. External use remains unapproved.

## EXT-05 Strict Reusable Fake-Codex Contract (2026-07-16)

- Task selected: `EXT-05`, one strict reusable fake-Codex contract fixture for
  the production autonomous app test path.
- The fixture is a separately built Go executable under app testdata. Its
  strict sibling contract names the exact version-call count, exec invocation
  order, argv, working directory, prompt, schema bytes, environment-name set
  and exact environment SHA-256, last-message bytes, JSONL event sequence, and
  optional receipt bytes. An atomically replaced sibling state records completed
  version calls, exec calls, and emitted event types.
- The positive contract performs one version probe followed by distinct
  supervisor and worker processes through `codexexec.Run` with its default
  `runner.Run`. Both calls require one fresh `exec --json --ephemeral`
  invocation, forbid `resume`, produce exact last-message and JSONL artifacts,
  and make the worker publish a parseable deterministic receipt. No model,
  network, injected `StepRunner`, command-runner replacement, or in-process
  Codex implementation is used.
- The permanent refusal matrix proves that unexpected argv, working directory,
  schema bytes, environment, invocation count, output-event sequence, and a
  missing ephemeral flag all exit with the fixture's refusal status. An extra
  call after the complete supervisor/worker sequence is also refused. Ambient
  environment values are never persisted in the contract; only sorted names
  and the exact sorted-environment SHA-256 are retained.
- Files changed for EXT-05: `internal/app/strict_fake_codex_test.go`,
  `internal/app/testdata/strictfakecodex/main.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on both new Go files; `go test -count=1
  ./internal/app -run '^TestStrictFakeCodexContract$'`; `go test -race
  -count=1 ./internal/app -run '^TestStrictFakeCodexContract$'`; `go test
  -count=1 ./internal/codexexec ./internal/runner`; `go test -count=1 ./...`;
  and `git diff --check`.
- Verification result: every required focused, race, owner-package, and full
  repository test passed. The focused test was iterated while implementing the
  fixture to correct schema-path and inherited `PWD` expectations; the final
  required matrix is green. No dependency, production behavior, or commit was
  added.
- What remains: EXT-06 and the later ordered external-readiness gates.
  Blockers for EXT-05: none. External use remains unapproved.

## EXT-04 Release-Authored Executable Identity Authority (2026-07-15)

- Task selected: `EXT-04`, exact release-authored Codex executable/version
  admission plus resolved Git executable identity projection and recording.
- An embedded, strict release manifest admits exactly `codex-cli 0.144.4` with
  SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  Executable inspection binds the configured spelling to its canonical
  symlink-resolved regular file and hashes the opened bytes while checking the
  named/opened identity remains stable. Exact version-and-digest equality is
  the only Codex authority; semantic ranges are rejected.
- Shared preflight records and renders the admitted Codex and Git identities.
  Autonomous execution rechecks both identities before fingerprinted task
  effects, and the Codex runner independently rechecks release authorization
  before creating artifacts or invoking the admitted resolved path. The
  identities flow through effective-config schema v7, supervisor/worker
  invocation provenance, and durable run evidence. Config check renders and
  fingerprints the observed exact identities while reporting release refusal
  separately, so it remains diagnostic for an installed but unlisted build.
- Files changed for EXT-04: `internal/codexexec/release_manifest.json`,
  `internal/codexexec/identity.go`, `internal/codexexec/invocation.go`,
  `internal/codexexec/codexexec.go`, `internal/codexexec/codexexec_test.go`,
  `internal/runonce/runonce.go`, `internal/runonce/effectiveconfig.go`,
  `internal/app/external_admission.go`, `internal/app/autonomous_run.go`,
  `internal/app/preflight.go`, `internal/app/config.go`,
  `internal/app/external_preflight_test.go`, `internal/app/app_test.go`,
  `internal/autonomouscycle/types.go`, `internal/autonomouscycle/cycle.go`,
  `internal/autonomouscycle/worker.go`, `internal/supervisor/execution.go`,
  `internal/cli/config.go`, `internal/cli/doctor.go`, `internal/cli/root.go`,
  `internal/cli/doctor_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`,
  this state file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on changed Go files; the required focused
  ordinary and race runs for
  `TestExternalExecutableIdentityAdmission|TestReleaseCodexAllowlist`; focused
  CLI regression tests; `go test -count=1 ./...`; `go run ./cmd/revolvr
  --help`; `go run ./cmd/revolvr doctor --help`; a built CLI `config check` in
  a clean temporary repository; and `git diff --check`.
- Verification result: all required focused, race, CLI, and full repository
  checks passed. The initial full run exposed a stale ready-doctor fixture with
  a noncanonical fake version; the single repair made it derive the exact
  release version, and the final full run passed. The first manual config-check
  fixture inherited unsafe Git directory permissions; repeating it with a
  restrictive fixture umask passed and showed matching Codex/Git identities
  and effective schema v7. No dependency or commit was added.
- What remains: EXT-05 and the later ordered external-readiness gates.
  Blockers for EXT-04: none. External use remains unapproved.

## EXT-03 Initial External Scope and Attended Bounds (2026-07-15)

- Task selected: `EXT-03`, initial repository, platform, verification, and
  attended operational-bound enforcement at shared preflight/no-model
  admission.
- The shared external-scope check resolves the configured Git executable,
  requires the requested root to be an operator-controlled non-bare Git
  worktree, rejects active submodules, requires a clean worktree and at least
  one verification command, and admits attended-task only on Linux, macOS, or
  FreeBSD while queue/daemon remain Linux-only. Public attended, queue, and
  daemon entry points run it before the execution lock, workspace, ledger,
  task, model, or verification effects.
- Level-1 effective configuration now carries finite task/action, elapsed,
  token, cycle, process, output, retained-disk, and notification-attempt
  bounds. The documented defaults are fingerprinted, rendered by config check
  and doctor, applied to existing attempt/cycle and subprocess authorities,
  and copied into durable task-operation and ledger evidence. Caller cycle
  overrides are recorded exactly and unlimited attended cycles are refused;
  retained-disk evidence derives from the effective retention operation cap.
- Files changed for EXT-03: `.agent/AUTONOMOUS_EXTERNAL_READINESS.md`,
  `internal/app/external_admission.go`, `internal/app/autonomous_run.go`,
  `internal/app/preflight.go`, `internal/app/config.go`,
  `internal/app/config_test.go`, `internal/app/app_test.go`,
  `internal/app/autonomous_scheduler_test.go`,
  `internal/app/external_preflight_test.go`,
  `internal/autonomoustaskrun/contracts.go`,
  `internal/autonomoustaskrun/ledger.go`,
  `internal/autonomoustaskrun/run.go`, `internal/runonce/effectiveconfig.go`,
  `internal/runonce/runonce.go`, `internal/cli/config.go`,
  `internal/cli/doctor_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`,
  this state file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on changed Go files; the required focused
  ordinary and race runs for
  `TestExternalRepositoryShapeAndPlatformMatrix|TestAttendedEffectiveBoundsVisibleAndRecorded`;
  focused affected-package tests; `GOOS=darwin go test -c ./internal/app`;
  `GOOS=freebsd go test -c ./internal/app`; `go test -count=1 ./...`; a built
  CLI `config check` in a clean temporary repository; `go run ./cmd/revolvr
  doctor --help`; and `git diff --check`.
- Verification result: all required focused, race, cross-compile, CLI, and full
  repository checks passed. Regressions prove the platform matrix, safe
  non-bare admission, bare/active-submodule/missing-verification/dirty/unresolved-
  Git refusal without model calls or runtime/task mutation, visible finite
  defaults, fingerprint inclusion, and exact durable operation evidence. A
  manual check also found and repaired omitted-working-directory resolution in
  config check. No dependency or commit was added.
- What remains: EXT-04 and the later ordered external-readiness gates.
  Blockers for EXT-03: none. External use remains unapproved.

## EXT-02 Mode-Aware Read-Only Doctor (2026-07-15)

- Task selected: `EXT-02`, the settled mode-aware, read-only doctor command
  surface.
- Bare `doctor` and `doctor --for attended-task` normalize to identical
  preflight input and output. `--for` accepts only `attended-task`, `queue`,
  and `daemon`; `--task <id>` is an exact selector admitted only for attended
  mode. Invalid modes, empty explicit flags, malformed selectors, and
  mode/selector conflicts return before repository inspection, external
  commands, or writes.
- Preflight now reports its normalized mode/task authority and runs the same
  strict autonomous graph loader used by direct task and queue execution. The
  loader validates every canonical task, required autonomous state, child
  publication lineage, archive authority, and graph diagnostic. An exact
  attended selector must identify an autonomous task currently classified
  `ready`. Existing execution paths reload this authority; preflight is not a
  lease.
- Files changed: `internal/app/preflight.go`,
  `internal/app/autonomous_run.go`, `internal/app/app_test.go`,
  `internal/app/external_preflight_test.go`, `internal/cli/doctor.go`,
  `internal/cli/doctor_test.go`, `.agent/TASKS.md`, this state file, and
  `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on every changed Go file; focused ordinary
  and race commands for
  `TestModeAwarePreflight|TestDoctorForModesAndTaskSelector`; complete app and
  CLI package tests; all existing preflight/doctor tests; `go run
  ./cmd/revolvr doctor --help`; `go test -count=1 ./...`; and `git diff
  --check`.
- Verification result: focused ordinary/race tests, affected package tests,
  CLI help, and the complete repository suite passed. Regressions prove all
  modes, bare/explicit byte equivalence, exact-task readiness, pre-command
  invalid-request refusal, unsafe protected-state and invalid-graph refusal,
  repository immutability, and execution recheck after authority drift. No
  dependency or commit was added.
- What remains: EXT-03 and the later ordered external-readiness gates.
  Blockers for EXT-02: none. External use remains unapproved.

## EXT-01 Shared Repository-Path Admission (2026-07-15)

- Task selected: `EXT-01`, shared repository-path admission agreement across
  doctor, status, canonical task loading, and no-model autonomous admission.
- `internal/repositorypath` now binds one canonical repository-root identity,
  validates present `.agent`, `.agent/tasks`, canonical Markdown task files,
  `.revolvr`, optional config, and ledger paths without creating anything, and
  retains protected descriptor reads/enumeration for consumers. Missing paths
  remain nonmutating presence facts; status initialization keeps its existing
  runtime-directory-plus-ledger meaning.
- Doctor and status run that inspection first. Unsafe doctor input yields one
  failed state check and no command; status refuses the same authority.
  Configuration and task bytes are read through the inspected root identity.
  Exact-task and queue entry points inspect before acquiring the global
  autonomous-execution lock, so refused no-model probes create no locks or
  runtime evidence.
- Files changed: `internal/repositorypath/repositorypath.go`,
  `internal/taskfile/taskfile.go`, `internal/app/app.go`,
  `internal/app/preflight.go`, `internal/app/config.go`,
  `internal/app/autonomous_run.go`,
  `internal/app/external_preflight_test.go`,
  `internal/cli/doctor_test.go`,
  `internal/autonomousmigration/plan_test.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on every changed Go file; focused ordinary
  and race commands for
  `TestExternalPreflightSharedPathMatrix|TestDoctorStatusAdmissionAgreeOnUnsafeAgent`;
  affected repositorypath/taskfile/app/CLI package tests; autonomous-migration
  package tests; `go test -count=1 ./...`; and `git diff --check`.
- Verification result: the focused ordinary and race regressions passed, as
  did the complete suite after one compatibility repair updated the migration
  symlink fixture to expect the new earlier shared refusal. The matrix proves
  safe/missing/wrong-type/final-symlink/ancestor-symlink/hard-link/group-write/
  identity-substitution behavior for both roots and exact repository/outside
  snapshot preservation on every refusal. No dependency or commit was added.
- What remains: EXT-02 and the later ordered external-readiness gates.
  Blockers for EXT-01: none. External use remains unapproved.

## External Readiness Backlog Decomposition (2026-07-15)

- EXT-01 through EXT-21 cover common and attended Level-1 authority:
  shared/mode-aware preflight, repository and executable scope, strict
  production fake-Codex composition, workspace/Git containment, explicit
  recovery, interruption proof, mandatory CI, candidate/runbook/evidence
  preparation, quantitative live dogfood, and an immutable release decision.
- EXT-22 through EXT-36 cover Level 2 only after Level-1 approval:
  explicit finite bounds and enforcement, deterministic stop policy, durable
  quarantine and unrelated-work continuation, production sequential queue
  composition, administrative recovery, exact unattended acknowledgement,
  rootless OCI isolation, retention/notification operations, the Level-2
  runbook and soak, and a separate tagged decision.
- EXT-37 through EXT-41 cover Level 3 only after Level-2 approval:
  self-wake exclusion, daemon interruption/restart recovery, the daemon
  runbook, the 72-hour quantitative soak, and a separate tagged decision.
- Each task names exact pass/fail behavior and either focused ordinary/race/full
  Go commands, concrete smoke/adversarial scripts, or immutable CI/build/
  dogfood/tag evidence. Quantitative thresholds and zero-tolerance soak
  failures are copied from the settled readiness policy rather than reopened.
- No production code, dependency, readiness policy, or decision was changed in
  this decomposition pass. Files changed by the pass are .agent/TASKS.md and
  this state summary. Backlog-decomposition blockers: none; external-use
  approval blockers remain those in
  .agent/AUTONOMOUS_EXTERNAL_READINESS.md.

## Final R4 Audit Closure (2026-07-14)

- AP-01: `runtimepath.Boundary` provides descriptor-rooted no-follow
  traversal plus identity-checked create, open, enumerate, link, replace,
  unlink, read, and sync operations. Autonomous state, notification,
  exact-task run, archive, and finalization owners retain stable parents and
  opened files across publication, cleanup, lease checks, and readback. The
  six named evidence readers and the additionally discovered migration reader
  use the same protected descriptor reads; the audited bespoke walkers and
  check-then-reopen reads are absent.
- Fresh AP-01 execution covered the original autonomous-state outside-mutation
  reproducer ten times, the shared boundary ten times, every other durable
  owner five times, and all seven evidence readers ten times. Final/ancestor
  symlinks, hard links, unsafe modes, renamed ancestors, metadata boundaries,
  cleanup, enumeration, held-lease replacement, and complete outside-tree
  contents, modes, targets, and link counts remain permanent assertions.
- AP-02: post-`SIGKILL` settlement has its own deadline, preserves signal and
  inspection errors, returns `ErrProcessTreeUnsettled` when absence cannot be
  proven, and checks leader identity reuse before every post-reap signal or
  poll. Ten focused runs included the real held-zombie descendant boundary.
- AP-03: active `q` and `ctrl+c` request cancellation but only the matching
  run token's fully applied terminal message emits `tea.Quit`. Twenty focused
  runs covered run-once, loop, exact-task, and queue cleanup and refresh.
- AP-04: explicit usage schemas have declared parent/key precedence; legacy
  recursive usage is accepted only when unique and otherwise returns typed,
  sorted ambiguity evidence without changing receipt bytes. Twenty focused
  runs covered every supported shape, precedence boundary, and 1,000 parses
  of the multi-candidate event per invocation.
- AP-05: regular and symlink source evidence is read through opened identities
  with immediate and post-read descriptor/path checks. Twenty focused runs
  covered final-symlink, inode, mutation, regular ABA, symlink ABA, and
  post-read substitution cases with replacement metadata equalized.
- AP-06: prior action budgets are sorted by action and archive byte checks use
  sorted expected paths. Twenty focused runs asserted exact multi-invalid
  first errors and archive object-read order across 1,000 calls per test.
- Final verification passed: complete ordinary, shuffled, and race suites;
  `go vet ./...`; `go mod verify`; `govulncheck ./...` with zero reachable
  vulnerabilities; tracked-Go formatting; `git diff --check`; Bash syntax for
  all tracked shell scripts; and CLI help. A fresh `umask 0022` Git fixture
  passed `init`, `config check`, and `status`; the existing working copy's
  group-writable `.agent` directory was correctly refused and left unchanged.
- Cross-builds passed for Linux 386/amd64/arm64, Darwin amd64/arm64, and
  FreeBSD amd64/arm64. Darwin and FreeBSD `gitstate` test binaries compile,
  and the Windows diagnostic stub contains the required unsupported-platform
  message. No dependency was added, the audit document is deleted, the
  backlog is empty, and blockers are none.

## Deterministic Remaining First-Error Diagnostics (2026-07-14)

- Attempt-transition validation copies and sorts prior action budgets by
  action before checking authority, consumption, and disappearance. This
  matches the action-budget canonicalization used by the attempt controller
  and removes map iteration from diagnostic selection.
- Archive commit verification already sorts the exact expected path set for
  commit comparison. Expected file bytes are now checked in that same order,
  while paths without an expected byte payload remain path-only evidence.
- Multi-invalid regressions supply action budgets in reverse order and assert
  the exact first error for two missing and two changed budgets. Archive
  regressions supply reverse paths with two missing or two byte-mismatched
  files and assert both the exact error and the first Git object read. Every
  case executes 1,000 times per test invocation.
- Verification passed: twenty focused repetitions, ten complete repetitions
  of both owner packages, owner race tests, complete ordinary/shuffled/race
  suites, `go vet ./...`, `go mod verify`, CLI help, formatting and diff
  checks, Linux/Darwin/FreeBSD amd64 builds, and the unsupported-Windows
  diagnostic-stub build. No dependency was added. The next task is
  `AUDIT-R4-CLOSE-01`; blockers: none.

## Descriptor-Bound Source Snapshot Entries (2026-07-14)

- Regular entries are opened with no final-component symlink following and
  nonblocking semantics. Immediate descriptor metadata must match the initial
  pathname identity, type, mode, size, and modification time before hashing.
- After hashing, both the opened descriptor and the pathname are checked
  against the opened identity before the digest is published. This prevents a
  pathname from supplying metadata for bytes read from a substituted inode.
- Symlinks are opened as link-inode descriptors and their targets are read
  through those descriptors: `O_PATH` plus descriptor-relative `readlinkat`
  on Linux and FreeBSD, and `O_SYMLINK` plus `freadlink` on macOS. Descriptor
  and pathname identity checks bracket the target read.
- Deterministic capture hooks reproduce final-symlink replacement, regular
  inode replacement, regular and symlink A-to-B-to-A substitutions, a
  regular-file mutation after hashing, and symlink replacement around and
  after the descriptor target read. Replacement metadata is deliberately
  matched so identity checks, not incidental size or timestamp differences,
  cause rejection.
- Verification passed: twenty focused adversarial repetitions, complete
  ordinary/shuffled/race suites, `go vet ./...`, `go mod verify`, CLI help,
  formatting and diff checks, Linux/Darwin/FreeBSD amd64 builds, Darwin and
  FreeBSD `gitstate` test-package builds, and the unsupported-Windows
  diagnostic-stub build. No dependency was added. The next task is
  `AUDIT-R4-11`; blockers: none.

## Codex Usage Schema Precedence (2026-07-14)

- Direct `usage`, `total_usage`, `token_usage`, and `total_token_usage`
  objects have highest authority in that order. The same key order under
  `response`, `result`, `message`, and `event` envelopes follows in that
  parent order, then bare event metric fields.
- The legacy arbitrary-depth shape is compatibility-only. The parser
  enumerates every metric-bearing named usage object, sorts RFC 6901-style
  candidate paths for diagnostics, and accepts it only when exactly one
  candidate remains. Traversal order never chooses among candidates.
- Competing legacy candidates return `CodexUsageAmbiguityError` with the
  record number and stable candidate paths and unwrap to
  `ErrCodexUsageAmbiguity`. File parsing classifies it as a parse diagnostic,
  not a source failure; Codex results publish no usage and receipt rewriting
  returns the original bytes unchanged.
- Regressions cover all 20 named usage-object paths, the bare-metrics and
  unique-legacy shapes, every schema-precedence boundary, receipt
  preservation, file classification, and 1,000 fresh parses of the reproduced
  multi-candidate event per test invocation.
- Verification passed: twenty focused repetitions, ten complete receipt-
  package repetitions, the receipt race suite, complete ordinary/shuffled/
  race suites, `go vet ./...`, `go mod verify`, CLI help, tracked-Go formatting
  and diff checks, Linux/Darwin/FreeBSD amd64 builds, and the unsupported-
  Windows diagnostic-stub build. No dependency was added. The next task is
  `AUDIT-R4-10`; blockers: none.

## Protected Missing-Receipt Assertion Repair (2026-07-14)

- `runtimepath.Boundary.ReadFileLimit` deliberately reports absent required
  files with `os.ErrNotExist`. The taskschedule regression now asserts that
  contract through `os.ErrNotExist.Error()` rather than the obsolete
  `no such file` literal produced by the former pathname read.
- Fifty focused repetitions, ten complete taskschedule-package repetitions,
  and `go test -count=1 ./...` passed. No production behavior or dependency
  changed. The next task is `AUDIT-R4-09`; blockers: none.

## TUI Quit-After-Settlement (2026-07-14)

- Active `q`/`ctrl+c` sets `QuitAfterSettlement` on the current token-bound run
  state, requests cancellation once, and returns no `tea.Quit`. The existing
  `c` key remains cancel-without-quit.
- Progress and terminal commands continue draining the operation's buffered
  message stream while quit is pending. Cancellation stops producers from
  blocking on progress sends, while the terminal send remains consumable by
  the outstanding start/wait command.
- Only a terminal message with the current operation token can release the
  delayed quit. The model first applies the mode-specific terminal result,
  refreshed status, run detail, logs, and inactive state, then clears the quit
  marker and emits `tea.Quit`. Stale terminal messages do neither.
- Autonomous task and queue completion normally reload workflow selectors;
  delayed quit intentionally takes precedence after terminal application,
  because the completion goroutine has already performed status refresh and
  durable domain cleanup.
- One table-driven regression covers run-once, bounded loop, exact task run,
  and autonomous queue under both `q` and `ctrl+c`. Each action blocks after
  observing cancellation, proves no quit or terminal before cleanup release,
  rejects a stale terminal, then proves cleanup and refresh precede the exact
  terminal and `tea.Quit`.
- Verification passed: twenty focused repetitions, ten complete TUI-package
  repetitions, focused and package race tests, complete ordinary/shuffled/race
  suites, `go vet ./...`, `go mod verify`, CLI help, formatting and diff checks,
  Linux/Darwin/FreeBSD amd64 builds, and the unsupported-Windows diagnostic
  stub. No dependency was added. The next task is `AUDIT-R4-09`; blockers:
  none.

## Bounded Post-Kill Process-Tree Settlement (2026-07-14)

- `runner.Command` has a distinct `KillSettlementPeriod`, defaulting to five
  seconds independently of the graceful TERM period. After the grace timer
  sends `SIGKILL`, the runner continues bounded liveness polling and cannot
  return until the original process group is proven gone.
- Failure to prove post-kill settlement returns `ErrProcessTreeUnsettled`.
  Graceful-signal, force-signal, and inspection errors remain joined with the
  cancellation/deadline cause instead of being discarded or replaced.
- One guarded lifecycle owns both signals and polls. Before every operation
  after `cmd.Wait` has reaped the leader, it checks whether that PID/process-
  group identity was reused. Natural-exit settlement and cancellation races
  share the same guard and fail closed without touching the replacement.
- A deterministic lower-level regression holds the post-kill liveness result
  until explicitly released. A Linux end-to-end regression adds a TERM-
  ignoring child to the command's process group, holds its killed zombie
  unreaped, proves `Run` has not returned, then reaps it and proves every tree
  member is non-executable and no sentinel exists beginning at return.
- Regressions also prove the separate kill deadline, typed unsettled failure,
  preservation of every error class, and refusal of natural-exit and
  cancellation identity-reuse races before platform signal/poll calls.
- Verification passed: ten complete runner-package repetitions, focused
  adversarial tests, runner race tests, complete ordinary/shuffled/race suites,
  `go vet ./...`, `go mod verify`, formatting and diff checks, Linux/Darwin/
  FreeBSD amd64 builds, and the unsupported-Windows diagnostic-stub build. No
  dependency was added. The next task is `AUDIT-R4-08`; blockers: none.

## Stable Remaining Evidence Readers (2026-07-14)

- `runtimepath` now provides capped reads through opened file descriptors.
  The limit is checked before allocation, enforced while reading, and followed
  by the existing named/opened inode recheck. A typed limit error lets owners
  preserve their established diagnostics.
- Audit apply, plan apply, operator checkpoint receipts, ledger export
  verification/replay, Codex last-message handling, and autonomous task views
  retain one repository or parent boundary across their authoritative reads.
  Exact hash/size validation and existing read caps remain in force.
- The inventory also found autonomous migration's exact orphan-state reader.
  Namespace enumeration and state comparison now share one opened directory
  and reject substituted ancestors, unsafe modes, aliases, and wrong types.
- Codex last-message handling retains its opened parent through raw read,
  redacted temporary write/sync, descriptor-relative replacement, raw cleanup,
  canonical mode readback, and directory sync. Cleanup can safely remove an
  owned unaliased inode after an external writer changes only its mode, while
  hard-linked or substituted files remain fail-closed.
- Permanent owner regressions bind first, replace an evidence ancestor with an
  outside symlink, and prove outside bytes and entries remain unchanged. The
  Codex regression exercises the complete `Run` path and failure cleanup.
- Verification passed: ten focused adversarial repetitions; all affected
  package tests and race tests; complete ordinary, shuffled, and race suites;
  `go vet ./...`; `go mod verify`; formatting and diff checks; Linux, Darwin,
  and FreeBSD amd64 builds; and the unsupported-Windows diagnostic-stub build.
  No dependency was added. The next task is `AUDIT-R4-07`; blockers: none.

## Stable Autonomous Finalization Artifact Boundary (2026-07-14)

- `Finalize` binds one `runtimepath.Boundary` for its complete transaction and
  routes frozen completion evidence, `completion.md`, and the completion
  manifest through one `finalizationStorage`. Replay verification uses the
  same retained root authority instead of re-resolving paths independently.
- Immutable completion artifacts are written and synced through an opened
  temporary inode and published with a descriptor-relative exclusive link.
  A racing destination is never overwritten: exact bytes are accepted as
  replay, conflicting or unsafe destinations fail closed.
- Publication retains the opened parent and temporary through post-link
  namespace checks, directory sync, and strict descriptor readback. Cleanup
  removes only an unpublished opened temporary from that parent; once the link
  syscall has completed the final name is recorded before later validation,
  so a published artifact is never treated as cleanup residue.
- Protected descriptor reads replace the finalization-specific parent walker,
  `Lstat`/`ReadFile`, `MkdirAll`, `CreateTemp`, pathname rename/removal,
  directory reopen/sync, and readback helpers. Canonical task and state changes
  remain with their already-hardened `taskfile` and `autonomousstate` owners.
- Permanent regressions reject final symlinks, hard links, unsafe modes,
  competing unsafe destinations, and renamed ancestors at every directory,
  temporary, sync, publication, readback, and cleanup boundary. They prove
  published-vs-unpublished state and exact outside entries, bytes, modes,
  symlink targets, and link counts.
- A full `Finalize` regression substitutes the completion namespace with
  same-named attacker temporary/final files and proves no outside mutation and
  no task, canonical-state, or ledger advancement. Focused adversarial tests
  passed ten repetitions and the package race suite.
- The complete ordinary, shuffled, and race suites, `go vet`, module
  verification, formatting/diff checks, CLI help, and Linux/Darwin/FreeBSD plus
  Windows-stub cross-builds passed. No dependency was added. The next task is
  `AUDIT-R4-06`; blockers: none.

## Stable Autonomous Archive Storage Boundary (2026-07-14)

- `Archive` and `Reopen` bind one `runtimepath.Boundary`, retain the actual
  Git-admin and archived-task state flocks, and route archive/runtime evidence
  through one `archiveStorage`. Locked reopen verification reuses that same
  store and proves the selected archive/task identity did not change during
  admission. Read-only list/show/load/verify operations retain one root
  authority for their complete traversal.
- Archive manifests, archived tasks/capsules, canonical state, frozen evidence,
  journals, history, and reopen records use protected descriptor reads.
  Archive enumeration recursively opens and reads children relative to
  retained directories; the bespoke symlink walker, `Lstat`/by-name reads,
  and `filepath.WalkDir` authority are removed.
- Immutable artifacts/history are written and synced through an opened
  temporary inode, published with a descriptor-relative exclusive link, then
  directory-synced and identity-read back. Mutable journals use retained-temp
  descriptor-relative replacement. Active-task removal unlinks only the
  opened, identity-matched file from its stable parent.
- Every directory/file open, enumeration, file/directory sync, publication,
  removal, cleanup, and readback boundary rechecks the stable namespace and
  held leases. Cleanup removes only an unpublished opened temporary; a
  completed metadata publication is never mistaken for cleanup authority, and
  namespace/lease loss cannot redirect cleanup outside the repository.
- Permanent regressions reject final symlinks, hard links, unsafe modes,
  ancestor replacement at every immutable/mutable/removal metadata boundary,
  enumeration substitution, and held-lock inode replacement. They install
  same-named attacker temporaries and prove exact outside entries, bytes,
  modes, symlink targets, and link counts remain unchanged. A full `Archive`
  regression also proves no active-task or Git-HEAD mutation after rejected
  persistence substitution.
- Focused adversarial tests passed ten repetitions and the package race suite.
  The complete ordinary, shuffled, and race suites, `go vet`, module
  verification, formatting/diff checks, CLI help, and Linux/Darwin/FreeBSD plus
  Windows-stub cross-builds passed. No dependency was added. The next task is
  `AUDIT-R4-05`; blockers: none.

## Stable Exact-Task-Run Persistence Boundary (2026-07-14)

- `RunTaskUntilTerminal` binds one `runtimepath.Boundary`, retains the opened
  operation and history directories, and keeps the actual exclusive
  `lock.Flock` for the complete operation. Admission, cycle, recovery, and
  terminal transitions all use that one store rather than re-resolving paths
  or reducing the lease to an unlock closure.
- Checkpoint/history recovery uses protected descriptor reads and stable
  directory enumeration. The old `CheckFile`/`CheckDir` followed by
  `os.ReadFile`/`os.ReadDir` pattern and its unused standalone load wrapper are
  removed.
- Immutable history now writes and syncs an opened temporary inode, publishes
  it with a descriptor-relative exclusive link, and syncs the retained history
  directory. Mutable `operation.json` retains its opened temporary through
  descriptor-relative replacement, operation-directory sync, and exact-byte
  readback.
- Every create, write/sync, history link, checkpoint replacement, cleanup,
  directory sync, and readback acceptance checks both the stable namespace and
  original operation lease. Cleanup unlinks only the still-opened temporary;
  namespace or lease loss leaves local residue instead of risking an outside
  unlink.
- Permanent end-to-end regressions substitute the operation directory before
  and after history/checkpoint open, publication, sync, cleanup, and readback;
  install same-named attacker temporaries; and prove the complete outside tree
  is unchanged including entries, bytes, modes, symlink targets, and link
  counts. Separate coverage rejects unsafe read evidence and replacement of
  only the held operation-lock inode.
- Focused adversarial tests passed ten repetitions and the package race suite.
  The complete ordinary and race suites, `go vet`, module verification,
  formatting/diff checks, CLI help, and Linux/Darwin/FreeBSD plus Windows-stub
  cross-builds passed. No dependency was added. The next task is
  `AUDIT-R4-04`; blockers: none.

## Stable Notification Persistence Boundary (2026-07-14)

- Notification delivery binds one `runtimepath.Boundary`, retains the opened
  delivery directory, and retains the hardened `lock.Flock` instead of
  converting it to an unlock closure. Every write, immutable link, journal
  replacement, cleanup unlink, sync, and readback acceptance rechecks the
  namespace and the held lease.
- Intent, payload, and transition history now write through opened temporary
  inodes and publish immutably with descriptor-relative exclusive links.
  `journal.json` uses a descriptor-relative atomic replacement. Both paths
  retain the opened temporary identity through post-publication checks and
  safely distinguish a completed metadata syscall from a pre-publication
  failure.
- History creation/opening is relative to the retained delivery directory.
  `Inspect`, `List`, journal reconstruction, and exact-content replay read and
  enumerate through stable directories and protected file descriptors; the
  notification-specific path walkers and `Lstat`/by-name reads are removed.
- Cleanup is fail-closed: it removes only the still-opened temporary from its
  stable parent while the original delivery lease remains valid. Namespace or
  lease loss preserves local residue and cannot unlink an attacker-selected
  outside entry.
- Deterministic before-open, after-open, before/after-publication, sync, and
  cleanup seams exercise intent, payload, history, and journal boundaries.
  Permanent regressions cover final symlinks, hard links, unsafe modes,
  renamed ancestors with same-named attacker temporaries, exact outside-tree
  contents/modes/link counts, and delivery-lock inode replacement.
- Focused tests passed repeatedly and under the race detector. The complete
  ordinary and race suites, `go vet`, module verification, diff/format checks,
  CLI help, and Linux/Darwin/FreeBSD plus Windows-stub cross-builds passed. No
  dependency was added. The next task is `AUDIT-R4-03`; blockers: none.

## Descriptor-Rooted Runtime And Autonomous-State Boundary (2026-07-14)

- `runtimepath.Boundary` binds the initially resolved repository root device
  and inode without retaining or leaking a descriptor. Every operation reopens
  that exact root and traverses descendants with no-follow `openat` calls,
  validating each opened/named identity and safe mode.
- Stable `Directory` and `File` handles now own descriptor-relative create,
  enumerate, exclusive link publication, atomic replacement, unlink, read,
  and directory/file sync. Existing package functions delegate to the same
  boundary rather than performing `Lstat` followed by a full-path operation.
- `autonomousstate.Store` retains one root identity, uses protected reads for
  canonical state and every planning/audit/attempt/input/block/optional-role/
  workspace/finalization history, and uses descriptor-relative immutable and
  state publication. The opened task directory and temporary inode remain
  bound across the locked CAS, rename, sync, and readback.
- Every production canonical-state replacement checks the held state flock
  immediately before publication and before accepting readback. Immutable
  state evidence checks the same lease before and after publication; cleanup
  proceeds only while the lease and stable namespace remain valid.
- The permanent original reproducer moves the real task directory aside,
  installs an outside symlink with attacker-controlled bytes under the same
  temporary basename, and proves replacement returns `ErrUnsafePath` without
  creating outside `state.json` or changing outside entries, contents, modes,
  or link counts. Shared-boundary regressions also cover ancestor replacement,
  cleanup refusal, exclusive link publication, and repository-root identity
  replacement.
- No dependency was added; the implementation uses the already-present
  `golang.org/x/sys/unix` module. Verification passed for focused and complete
  Go suites. Race, vet/module, diff, and supported-platform compile evidence is
  recorded before the implementation commit. The next task is `AUDIT-R4-02`;
  blockers: none.

## Fresh R4 Wide-Sweep Audit (2026-07-14)

- AP-01 is a systemic check/use filesystem gap. A temporary deterministic
  regression used `FailureBeforeStateRename` to replace the task-state
  namespace and proved that `replaceState` created an outside `state.json`
  from attacker-controlled bytes before returning an error. The same root
  pattern affects shared runtime-path creation and multiple state,
  notification, task-run, archive, finalization, and evidence-read owners.
- AP-02 proves the runner returns immediately after sending process-group
  `SIGKILL` without polling for exit. AP-03 proves active TUI `q`/`ctrl+c`
  emits `tea.Quit` before the background operation publishes its terminal
  message or finishes durable cleanup.
- AP-04 was experimentally reproduced: the same event with two nested usage
  objects yielded both token totals across 1,000 parses in each of ten focused
  runs. AP-05 identifies missing opened-file identity in authoritative source
  snapshots. AP-06 identifies two remaining map-order first-error diagnostics.
- Audit-only reproducer tests were removed. No production code or dependency
  changed. No speculative performance or LOC issue was recorded; consolidating
  corrected filesystem ownership is the one substantiated safe simplification
  opportunity.
- Fresh verification passed: ordinary, race-enabled, and shuffled complete Go
  suites; `go vet`; module verification; vulnerability scan with no reachable
  vulnerability; formatting/diff/shell checks; CLI help/config/status; and the
  supported Linux/Darwin/FreeBSD cross-build sample.
- The audit revision is `f9dcd960b3b4`. Follow-up tasks `AUDIT-R4-01` through
  `AUDIT-R4-11` are bounded by owner or behavior; blockers: none.

## Final R3 Audit Closure (2026-07-14)

- AP-01: current runner code inspects and settles the original process group
  after leader exit, refuses reused leader identities, preserves cancellation
  authority, and reports remaining descendants as a lifecycle error. Five
  repetitions each proved redirected natural-exit and cancelled writers cannot
  mutate after return.
- AP-02: current init code canonicalizes one worktree, preflights protected
  components before creation, uses no-follow named/opened identities, validates
  Git-reported admin/common/exclude paths, and accepts only reciprocal linked
  worktrees. Three focused repetitions covered hostile runtime, agent, task,
  profile, ledger, `.git`, and exclude components, post-open replacement, no
  outside mutation, and genuine common-exclude behavior.
- AP-03 through AP-07: twenty busy/cancellation repetitions retained typed
  SQLite evidence and reopened cleanly; fence unit/receipt/CLI regressions
  passed five times; SHA-1/SHA-256 validators and a real SHA-256 dossier passed
  three times; component-aware safe/traversal path and real-Git dossier tests
  passed five times; and all three multi-invalid diagnostic tests passed 100
  package repetitions.
- AP-08: the admitted-cycle file and all six audited symbols remain absent.
  The affected autonomous lifecycle packages passed three times, and complete
  repository compilation proves no consumer remains.
- Final verification passed: the complete suite twice after all production
  fixes, the original AP-03 shuffle seed, the complete race suite, `go vet
  ./...`, `go mod verify`, tracked-Go formatting, `git diff --check`, shell
  syntax, CLI help/config/status, all three documented non-Codex smoke tests,
  Linux/Darwin/FreeBSD amd64 CI-equivalent builds, and the Windows diagnostic
  stub build/message check.
- The first closure run exposed host-umask sensitivity in both fake-Codex smoke
  fixtures: their Git directories inherited mode `0775`, which AP-02 correctly
  rejects. Both scripts now set `umask 0022`; their complete success and
  expected-verification-failure paths then passed without weakening init.
- Files changed in closure: the two fake-Codex smoke scripts, durable agent
  state, and deletion of `AUDIT_PROBLEMS.md`. No dependency was added, nothing
  remains, and there are no blockers.

## Confirmed No-Caller Code Removal (2026-07-14)

- Repository-wide Go references prove `bytesSHA`, `persistTransition`, and
  `replaceFile` had definitions but no calls. Their three thin wrappers were
  removed; active notification paths continue to use the fault-aware
  persistence functions directly.
- `AdmittedCycleConfig`, `AdmittedCycleResult`, and `RunAdmittedCycle` had no
  source, test, documentation, or historical caller. The internal-only
  `admitted_cycle.go` prototype was removed; current app orchestration retains
  the live worker admission and completion path.
- Verification passed: affected packages once, five shuffled repetitions,
  affected-package race tests, `go test -count=1 ./...`, `go vet ./...`,
  `go mod verify`, formatting, `git diff --check`, a repository-wide absence
  check for every deleted symbol, Linux/Darwin/FreeBSD amd64 cross-builds, and
  the Windows diagnostic-stub cross-build.
- No dependency was added. The next task is `AUDIT-R3-CLOSE-01`; blockers:
  none. `AUDIT_PROBLEMS.md` remains until that independent closure pass.

## Deterministic Validation Diagnostics (2026-07-14)

- Source-snapshot hash validation now has explicit `index`, `worktree`, then
  `snapshot` precedence. Dossier-cache root validation explicitly checks the
  control root before the execution root.
- Audit report application collects every open finding omitted from the
  current report, sorts the IDs, and reports the lexically first missing ID.
  Map iteration is no longer diagnostic authority at any of the three audited
  boundaries.
- Each regression supplies multiple simultaneous invalid values and asserts
  one exact error over 100 calls. Focused packages passed ten ordinary and ten
  shuffled repetitions; the exact regressions passed 100 package repetitions,
  and affected-package race tests passed.
- Verification passed: `go test -count=1 ./...`, `go vet ./...`,
  `go mod verify`, formatting, `git diff --check`, Linux/Darwin/FreeBSD amd64
  cross-builds, and the Windows diagnostic-stub cross-build.
- No dependency was added. The next task is `AUDIT-R3-08`; blockers: none.

## Component-Aware Repository Paths (2026-07-14)

- Dossier-cache guidance and Git-tree paths now share one escape check. Only
  exact `..` or a cleaned path beginning with `..` plus the platform separator
  is traversal; a textual `..` prefix inside a legitimate component is inert.
- Existing empty, absolute, normalization, UTF-8, NUL, ordering, duplicate,
  mode/type, and protected `.git`/`.revolvr` checks remain unchanged.
- Unit regressions admit `..foo` and `..well-known/file` through both source
  guidance and repository-map construction. They reject `..`, `../foo`,
  `a/../../b`, an absolute path, `./foo`, `a/../b`, and `a//b` at both
  boundaries.
- A real Git fixture commits both safe names and proves they survive
  `ls-tree -z`, map construction, and complete autonomous dossier assembly.
- Verification passed: focused and complete affected-package tests, shuffled
  affected-package tests, ten race-enabled focused repetitions,
  `go test -count=1 ./...`, `go vet ./...`, `go mod verify`, formatting,
  `git diff --check`, Linux/Darwin/FreeBSD amd64 cross-builds, and the Windows
  diagnostic-stub cross-build.
- No dependency was added. The next task is `AUDIT-R3-07`; blockers: none.

## SHA-1 and SHA-256 Git Object Identities (2026-07-14)

- `internal/gitoid.Valid` is the sole Git object-ID grammar: exactly 40 or 64
  lowercase hexadecimal characters. Abbreviations, uppercase, non-hex, and
  whitespace-padded values fail closed.
- Task workspace/state, workspace-manager observations, archive and
  finalization evidence, dossier-cache sources, NUL-delimited `git ls-tree`
  records, and repository-map dossier projections all delegate to that shared
  contract. The cache and projection diagnostics now describe both supported
  lengths.
- Regressions validate both SHA-1 and SHA-256 forms and reject invalid
  length/case/hex values at the shared and `ls-tree` boundaries. Workspace,
  cache-source, and dossier-projection tests cover both supported forms.
- A real `git init --object-format=sha256` fixture executed successfully in the
  current environment. Its 64-character HEAD, tree, and `ls-tree -z` object
  IDs survive assembly, cache miss/publication, repository-map construction,
  dossier projection, and cache-hit replay.
- Verification passed: focused and complete affected-package tests, shuffled
  affected-package tests, five race-enabled focused repetitions,
  `go test -count=1 ./...`, `go vet ./...`, `go mod verify`, formatting,
  `git diff --check`, Linux/Darwin/FreeBSD amd64 cross-builds, and the Windows
  diagnostic-stub cross-build.
- No dependency was added. The next task is `AUDIT-R3-06`; blockers: none.

## Fence-Aware Markdown Structure (2026-07-14)

- `internal/markdown.Fence` is the shared fence state machine for task imports
  and receipts. It recognizes backtick and tilde fences with up to three
  leading spaces, requires a same-marker closing fence at least as long as the
  opener, accepts longer closers and CRLF, and keeps an unclosed fence active
  through EOF.
- Task-import headings are structural only outside a fence. Fence markers and
  contents remain in the imported task text, while a real heading immediately
  after a valid closer is recognized normally.
- Receipt required-section discovery, claim-section extraction, and
  harness-owned section rewriting use the same fence state. Fenced `## Changed
  Files` and `## Verification` examples cannot satisfy required sections,
  redirect claims, or receive harness rewrites; code-block claim extraction
  retains its existing behavior for actual receipt sections.
- Regressions cover both marker types, 0–3-space indentation, shorter inert
  closers, longer valid closers, unclosed fences, CRLF, headings immediately
  after closure, exact fenced-example preservation, and an actual CLI import
  that creates exactly one task from the original reproducer shape.
- Verification passed: focused tests, all affected-package tests, shuffled
  affected-package tests, ten race-enabled focused repetitions,
  `go test -count=1 ./...`, `go vet ./...`, `go mod verify`, root help,
  formatting, and `git diff --check`.
- No dependency was added. The next task is `AUDIT-R3-05`; blockers: none.

## Live-Reader Busy Evidence Retention (2026-07-14)

- A live-read retry now retains the most recent SQLite busy/locked error for
  the full operation. If a later attempt returns cancellation/deadline or the
  context terminates around another error, the read returns its type's zero
  value and joins the context cause with that retained SQLite evidence.
- An ordinary non-context error still returns directly, and a context failure
  with no preceding busy attempt contains no fabricated SQLite evidence. Busy
  retry exhaustion continues to return the latest SQLite error.
- The former 50ms scheduling-dependent regression now captures one real
  rollback-journal busy error, then scripts later deadline, cancellation, and
  no-busy outcomes through the retry operation seam. It checks zero results,
  `errors.Is` context causes, `errors.As` SQLite evidence, latest-only busy
  retention, and successful same-reader and reopened-reader snapshots.
- Verification passed: the focused regression once and twenty consecutive
  times, the original shuffled seed across `./...`, twenty race-enabled
  focused runs, `go test ./...`, `go vet ./...`, formatting, and
  `git diff --check`.
- No dependency was added. The next task is `AUDIT-R3-04`; blockers: none.

## Initialization Filesystem Trust Boundary (2026-07-14)

- CLI state paths canonicalize the worktree once. Init performs a read-only
  preflight of every existing `.revolvr`, `.agent`, profile, task, ledger, and
  Git component before its first repository mutation; symlinks, wrong types,
  group/other-writable paths, hard links, and escaping identities fail closed.
- `runtimepath.OpenFile` is the shared writable-open primitive. It validates
  ancestors and the final component, opens with no-follow/nonblocking flags,
  proves the opened/named regular-file identity, and forbids pre-validation
  truncation. Profiles, the guarded ledger identity, Git exclude updates, and
  new task files use this boundary and recheck identity after writes.
- Git administrative paths come from bounded `git rev-parse` output. The
  reported top-level, Git directory, common directory, and common
  `info/exclude` must agree with the canonical worktree and protected path
  identities. External gitdir files are accepted only for genuine linked
  worktrees with a canonical `worktrees/<name>` admin path and matching
  backlink; forged pointers and per-worktree pseudo-excludes are rejected.
- The exclude updater appends through an already identity-checked descriptor.
  A path replacement after open is detected before use and cannot modify the
  outside replacement target.
- `writeNewTaskFile` validates containment through `runtimepath.EnsureDir`
  before creating missing directories and uses exclusive no-follow creation
  plus file sync and opened/named identity revalidation.
- Regressions cover `.revolvr`, `.agent`, profile/task directories, the final
  profile/task and ledger files, `.git` symlinks and forged gitdir files,
  `info/exclude`, unsafe modes/types, opened-file substitution, and a genuine
  linked worktree. Rejected init fixtures leave both repository and outside
  tree snapshots unchanged.
- Verification passed: five repeated focused runs, focused race tests, package
  tests, `go test -count=1 ./...`, focused `go vet`, tracked-Go formatting,
  `git diff --check`, root help, config check, status, supported Unix
  cross-builds, and the unsupported Windows diagnostic-stub build.
- No dependency was added. The next task is `AUDIT-R3-03`; blockers: none.

## Runner Process-Group Settlement (2026-07-14)

- `runner.Run` now inspects the original process group after `cmd.Wait` on
  every natural-exit path. Remaining descendants receive the existing bounded
  TERM/poll/KILL settlement before the runner returns.
- A leader that exits while descendants remain produces the distinct
  `ErrProcessTreeUnsettled` runner error even when its own exit code is zero;
  cancellation and deadline causes retain their existing result authority.
- Post-exit signalling first checks that no process has reused the reaped
  leader PID. If reuse is observed, the runner fails closed without signalling
  the unrelated group identity.
- Unix regressions use a direct shell child whose background writer redirects
  every inherited stream. Natural leader exit is not reported as success, and
  natural and cancelled runs cannot perform the delayed sentinel mutation
  after runner return.
- Verification passed: runner package test, five repetitions of six focused
  lifecycle tests, runner race test, `git diff --check`,
  `go test -count=1 ./...`, and
  Linux/Darwin/FreeBSD amd64 plus unsupported-Windows-stub cross-builds.
- No dependency was added. The next task is `AUDIT-R3-02`; blockers: none.

## Independent Wide-Sweep Audit (2026-07-14)

- Two high-severity boundary problems were reproduced: successful runner
  leaders can leave mutation-capable descendants alive, and `revolvr init` can
  follow repository-controlled symlinks and write outside the repository.
- A shuffled run and a focused invocation configured for twenty repetitions
  exposed loss of SQLite busy evidence when a live-reader context expires
  during a later retry.
- A CLI fixture proved that a task heading inside a fenced Markdown example is
  persisted as a second task. Receipt section scanners share the same issue.
- Direct code and Git probes found partial SHA-256-repository support, rejection
  of safe names beginning with `..`, three map-order-dependent diagnostics, and
  a small set of confirmed no-caller production code.
- Ordinary tests, race tests, `go vet`, module verification, formatting, shell
  syntax, local smokes, supported cross-builds, and CLI help passed.
  `govulncheck` found no reachable vulnerability. The shuffled/focused ledger
  failure is retained as audit evidence rather than hidden.
- No production code was changed, no dependency was added, and no commit was
  created during the audit.

## Final Audit Closure (2026-07-14)

- Task selected: `AUDIT-CLOSE-01`.
- Re-audit result: the source-lease settlement race, dirty-worktree commit
  escape, predictable coordinator lock identities, writable app ledger reads,
  inconsistent platform contract, and stale smoke header are all closed by
  their committed production boundaries and regression tests.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and deletion of
  `AUDIT_PROBLEMS.md`.
- Focused verification commands run: 25 repeated source-guard/run-once/
  autonomous-cycle race tests; real-Git dirty-worktree refusal; shared,
  source-writer, retention, Git-admin, and delivery lock path/substitution
  tests; immutable app-ledger projection tests; shell syntax and local smoke.
- Final verification commands run: tracked-Go `gofmt -l`, `git diff --check`,
  `go mod verify`, `go vet ./...`, `govulncheck ./...`, syntax checks for every
  tracked shell script, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, `go test -shuffle=on -count=1 ./...`, root
  help, config check, all three smoke scripts, Linux/Darwin/FreeBSD amd64
  cross-builds, and the Windows diagnostic-stub build/message check.
- Verification result: every focused and final check passed; `govulncheck`
  reported no reachable vulnerabilities. No live Codex execution was started
  through Revolvr.
- What remains: no backlog task. Blockers: none.

## Closed Wide-Sweep Audit (2026-07-14)

- Six evidence-backed problems were found: a cancellation/source-lease race,
  unsafe staging when pre-existing dirty work is allowed, unprotected
  coordinator lock-file identities, writable ledger opens in read projections,
  an unstated/inconsistent non-Unix build contract, and a stale local smoke-test
  assertion.
- No speculative performance or line-count cleanup was recorded.
- Final closure re-audited every finding against production code and focused
  regressions. Twenty-five repeated deterministic race tests passed, including
  canonical task, receipt, ledger, result, and single-release assertions.
- Final ordinary, race-enabled, and shuffled suites passed. The local smoke and
  both fake-Codex run-once smokes passed.
- Linux, Darwin, and FreeBSD builds passed. The intentional Windows diagnostic
  stub built successfully and retained its unsupported-platform message.

## Latest Completed Program (2026-07-14)

- One shared scheduler now owns graph validation, dependency semantics,
  deterministic ready ordering, and selected-next identity for mixed-pass and
  autonomous consumers. App, CLI, TUI, queue, and run-once projections agree
  and invalid graphs never fall back to an arbitrary pending task.
- Canonical task frontmatter is strict. Recognized fields retain their existing
  byte-preserving behavior; unsupported keys fail with exact source locations;
  and inert `x-` extensions survive metadata updates without affecting policy.
- `operator-checkpoint-v1` provides strict, receipt-bound external acceptance
  evidence. Fulfillment is atomic and replay-safe, never invokes Codex or
  commits, and unlocks dependents only through shared scheduler reevaluation.
- Autonomous migration supports deterministic all-or-nothing planning,
  dry-run inspection, locked state-before-task application, immutable recovery
  history, exact replay, orphan adoption, and conflict-safe restart behavior.
- Cross-surface regressions cover scheduler determinism, lifecycle and archive
  dependency states, invalid-graph/no-runner behavior, no-pending fallback, and
  task-list/status/TUI/run-once selection agreement.
- Operator documentation now covers canonical metadata, shared scheduling,
  checkpoint safety, autonomous `needs_input`, and dry-run-first migration and
  recovery.

## Cyber-ARPG Readiness Assessment

The read-only assessment is recorded in
`.agent/CYBER_ARPG_READINESS_ASSESSMENT.md`. It strictly loaded all 446 tasks
and 1,113 dependency edges, found no missing or duplicate identities/edges,
self-edges, cycles, terminal-unsatisfied edges, or scheduler diagnostics, and
selected `m0-architecture-dependency-guard` from two ready tasks. Task list,
status, and TUI agreed. Migration was dry-run only. The target repository's
HEAD, Git status, task/content manifests, and ignored runtime-state manifest
remained unchanged.

## Verification Baseline

- `gofmt` reports no unformatted Go files.
- `go test ./...` passes after app live-ledger projections were consolidated on
  the read-only opener and immutable-filesystem regressions were added.
- `go test -race -count=1 ./...` and
  `go test -shuffle=on -count=1 ./...` pass.
- `go vet ./...`, `go mod verify`, and `govulncheck ./...` pass; no reachable
  vulnerabilities were reported.
- `git diff --check` passes.
- Linux, Darwin, and FreeBSD amd64 CLI cross-builds pass; the unsupported
  Windows diagnostic stub also cross-builds and retains its failure message.
- Root help, config check, the local CLI smoke, and both fake-Codex run-once
  smokes pass.
- No live Codex execution was started through Revolvr during the final
  scheduling/checkpoint/migration campaign, readiness assessment, or audit.

## Durable Documentation

- `README.md`: operator setup, commands, workflows, and safety guidance.
- `AGENTS.md`: repository working and verification rules.
- `.agent/HANDOFF.md`: active pause/resume point, exact next command, and
  compact release authority needed to continue safely.
- `.agent/TASKS.md`: current backlog and compact completed-program index.
- `.agent/DECISIONS.md`: durable architecture and implementation decisions.
- `.agent/LOOP_PROMPT.md`: reusable fresh-session pass instructions.
- `.agent/CYBER_ARPG_READINESS_ASSESSMENT.md`: bounded external readiness
  evidence.

Obsolete setup handoffs, design-target drafts, and completed-program kickoff
notes and the resolved wide-sweep audit report were removed after their durable
content was incorporated into the files above. Detailed historical prose
remains available through Git history.

## Current Repository Understanding

- Stack: Go 1.22 CLI application.
- Entry point: `cmd/revolvr/main.go`.
- Build command: `go build ./cmd/revolvr`.
- Test command: `go test -count=1 ./...`.
- Formatting: `gofmt` on changed Go files.
- Runtime state: `.revolvr/`, local and ignored by Git.

## Verification Gaps

Remote GitHub Actions execution of the newly linted workflow has not occurred
in a local pass. The matching local test and cross-build matrix passes.

## Notes For Next Fresh Session

- Read `AGENTS.md` and `.agent/HANDOFF.md` first, then `.agent/TASKS.md`,
  `.agent/STATE.md`, and `.agent/DECISIONS.md` before acting.
- Resume with the exact command recorded in `.agent/HANDOFF.md`; at this
  boundary it is `./agent-ext20-rc3-attestation.sh`.
- Do one bounded task, verify it, update durable state, and stop.
- Do not use `codex resume` or depend on an old session transcript.
