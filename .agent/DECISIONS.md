# Agent Decisions

## RC.15 Failed Level-1 Dogfood; Planning Uses Exact Pre-Accounting State (2026-07-31)

- RC.15 remains a valid reproducibly constructed bundle, but it failed the
  quantitative Level-1 gate and is permanently ineligible for external-use
  approval. Its first live operation is immutable typed failure evidence at
  `.revolvr/ext20-level1-rc15-quantitative-20260731`; it may not be retried,
  removed, relabeled, or combined with evidence from a later candidate.
- The failure occurred after a successful read-only planner cycle and durable
  attempt admission/completion, before any source mutation. The planner dossier
  correctly bound the state that existed when the cycle began, while planning
  application incorrectly required it to equal canonical state after attempt
  accounting. This is a production coordination defect, not a model-output,
  collector, containment, candidate-identity, or external-sentinel failure.
- Planning application now follows the existing audit coordination rule: the
  caller supplies the exact state used to construct the dossier. That state
  must be a valid predecessor of current canonical state, and replacing current
  attempt accounting with dossier attempt accounting must make the states
  exactly equal. Thus only append-only attempt admission/completion can occur
  between dossier construction and application; any plan, lifecycle,
  workspace, decision, input, finding, terminal, or other difference remains a
  hard `dossier_identity` refusal.
- Because this changes product behavior, all later dogfood must use a new
  source-bound candidate that repeats candidate construction/verification and
  exact remote CI evidence. The verified RC.15 failure cannot be repaired in
  place and no later run may reuse its suite root or operation identity.
- Independent operator review reproduced the exact failed-live state boundary,
  accepted the narrowly scoped planning repair, and explicitly authorized its
  raw-Git publication. This acceptance grants no RC.15 retry, candidate,
  dogfood, release, or external-use authority.

## Reusable Level-1 Candidate Authority (2026-07-30)

- `scripts/build-level1-candidate.sh` is the reusable Level-1 candidate
  construction and verification workflow. Candidate-specific top-level wrapper
  scripts are not authority. Build mode requires a clean exact source commit
  and tree, and the executing workflow bytes must equal
  `scripts/build-level1-candidate.sh` at that commit before an output root may
  be created.
- A successful bundle is anchored by `candidate-authority.tsv` plus an
  externally supplied SHA-256. That authority fixes the candidate ID/version,
  source commit/tree, workflow hash, dogfood Linux binary path/hash, bundle-
  manifest hash, floor/current Go versions, and govulncheck executable hash.
  The bundle manifest is a complete regular-file inventory; verification
  rejects symbolic links, hard links, unlisted files, changed metadata, changed
  artifacts, nonempty build IDs, or non-identical independent builds.
- The quantitative Level-1 suite no longer owns a hard-coded historical
  candidate identity. Every suite mode requires `--candidate-authority` and
  `--candidate-authority-sha256`; preparation persists both, and later live or
  verification runs must present the same exact authority. Candidate
  construction and live Codex dogfood remain separate fresh-pass tasks.
- Build failures after output-root admission are retained with typed failed
  status and evidence. Successful construction uses two no-local clones,
  isolated Go build/module caches, explicit effective build settings, the Go
  1.22 source-floor test, current ordinary/race/module/vet checks, ordinary and
  verbose govulncheck, and byte-identical Linux/macOS/FreeBSD amd64 builds.
  Verification mode and suite static mode are read-only and never invoke Codex.

## Top-Level Agent Wrapper Policy (2026-07-30)

- Reusable operational behavior belongs under `scripts/`. Repository-top-level
  entrypoints are exceptional and remain only when they are genuinely reusable
  or are current authority. Retired one-shot development-control wrappers are
  preserved by Git history, not indefinitely in `HEAD`; they must not be moved
  to an archive directory.
- New candidate construction, verification, review, or other validation must
  never depend on a retired wrapper being present or absent. Historical wrapper
  names in durable prose remain historical facts, not executable dependencies
  or current authority. This decision supersedes older instructions that a
  tracked wrapper itself must remain in the current tree, without changing the
  recorded facts about what the wrapper did or which evidence it created.
- The read-only tracked-file audit classified all 69 top-level `agent-*.sh`
  files as follows:
  - **Currently referenced reusable infrastructure (2, retained):**
    `agent-one.sh`, the generic one-fresh-`codex exec` entrypoint required by
    the repository loop model; and `agent-loop.sh`, its interactive bounded
    multi-pass driver. `agent-loop.sh` executes `agent-one.sh`.
  - **Required current authority (0):** none. EXT-20 has no current wrapper
    authority, and its next gate is a reusable workflow under `scripts/`.
  - **Uncertain (0):** none.
  - **Retired historical wrappers (67, removed from `HEAD`):**
    `agent-attended-alpha-readiness.sh`, `agent-ext19.sh`,
    `agent-ext20-lifecycle-remediation.sh`,
    `agent-ext20-planner-contract-review.sh`, `agent-ext20-rc10.sh`,
    `agent-ext20-rc11.sh`,
    `agent-ext20-rc12-builder-publication-review.sh`,
    `agent-ext20-rc12-builder-publication.sh`,
    `agent-ext20-rc12-builder-revalidation-v3-review.sh`,
    `agent-ext20-rc12-builder-revalidation-v3.sh`,
    `agent-ext20-rc12-builder-revalidation-v4-review.sh`,
    `agent-ext20-rc12-builder-revalidation-v4.sh`,
    `agent-ext20-rc12-builder-revalidation.sh`,
    `agent-ext20-rc12-builder-validation-review.sh`,
    `agent-ext20-rc12-builder-validation.sh`,
    `agent-ext20-rc12-construction-failure-review.sh`,
    `agent-ext20-rc12-volatile-root-recovery-review.sh`,
    `agent-ext20-rc12.sh`,
    `agent-ext20-rc13-builder-revalidation-v6-review.sh`,
    `agent-ext20-rc13-builder-revalidation-v6.sh`,
    `agent-ext20-rc13-builder-revalidation-v7.sh`,
    `agent-ext20-rc13-builder-validation.sh`,
    `agent-ext20-rc2-attestation.sh`, `agent-ext20-rc2-suite.sh`,
    `agent-ext20-rc2.sh`, `agent-ext20-rc3-attestation.sh`,
    `agent-ext20-rc3-suite.sh`, `agent-ext20-rc3.sh`,
    `agent-ext20-rc4-attestation.sh`, `agent-ext20-rc4-live-direct.sh`,
    `agent-ext20-rc4-live.sh`, `agent-ext20-rc4-remote.sh`,
    `agent-ext20-rc4-suite.sh`, `agent-ext20-rc4.sh`,
    `agent-ext20-rc5-attestation.sh`, `agent-ext20-rc5-live-direct.sh`,
    `agent-ext20-rc5-live-failure-remediation.sh`,
    `agent-ext20-rc5-no-start-publish.sh`,
    `agent-ext20-rc5-no-start-remediation.sh`,
    `agent-ext20-rc5-no-start-review.sh`,
    `agent-ext20-rc5-recovery-publish.sh`,
    `agent-ext20-rc5-recovery-review.sh`, `agent-ext20-rc5-remote.sh`,
    `agent-ext20-rc5-suite.sh`, `agent-ext20-rc5.sh`,
    `agent-ext20-rc6-attestation-remote.sh`,
    `agent-ext20-rc6-attestation.sh`, `agent-ext20-rc6-live-direct.sh`,
    `agent-ext20-rc6-remote.sh`, `agent-ext20-rc6-suite.sh`,
    `agent-ext20-rc6.sh`, `agent-ext20-rc7-attestation-remote.sh`,
    `agent-ext20-rc7-attestation.sh`,
    `agent-ext20-rc7-await-live-authorization.sh`,
    `agent-ext20-rc7-live-authority-review.sh`,
    `agent-ext20-rc7-live-authorization-checkpoint.sh`,
    `agent-ext20-rc7-live-direct.sh`, `agent-ext20-rc7-live-gate.sh`,
    `agent-ext20-rc7-live-once.sh`, `agent-ext20-rc7-prelive-review.sh`,
    `agent-ext20-rc7-remote.sh`, `agent-ext20-rc7-suite.sh`,
    `agent-ext20-rc7.sh`, `agent-ext20-rc8.sh`, `agent-ext20-rc9.sh`,
    `agent-ext20-source-lock.sh`, and `agent-ext20.sh`.
- Exact-name and execution-pattern searches across Go code, scripts, CI,
  documentation, durable state, and all other tracked files found only
  historical prose, references among the retired wrappers themselves, and
  three executable RC-specific checkers. The latter—
  `scripts/check-ext20-rc5-live-direct.sh`,
  `scripts/check-ext20-rc6-live-direct.sh`, and
  `scripts/check-ext20-rc7-live-direct.sh`—had no callers and depended on the
  retired launchers plus ignored historical evidence, so they are retired and
  removed in the same cleanup. No `.revolvr/` path is changed by this policy.
- EXT-20 now proceeds in three bounded stages without another RC-numbered
  wrapper lineage: first implement one tracked, reusable, parameterized
  Level-1 candidate build-and-verification workflow under `scripts/`; next use
  it once to build and verify one fresh candidate; then run the existing
  quantitative Level-1 dogfood gate against that exact candidate. The first
  stage may make the existing dogfood suite consume parameterized candidate
  authority if needed, but it must not build a candidate or call live Codex.

## Prospective RC.13 V7 Design Is Rejected (2026-07-30)

- The sole v7 root is
  `.revolvr/prospective-builder-validation-v7.hYcT7T`. It is unsealed rejected
  evidence and cannot be repaired, rerun, sealed, copied, derived from,
  relabeled, removed, published as a builder, or submitted through the passing-
  design review path.
- A pre-sequence preservation bootstrap exposed one `set -u` same-command
  local expansion. The one permitted repair retained the failure evidence and
  split that declaration. Repaired sequence one then passed five gates before
  an analogous unbound `rel` expansion stopped the success publication probe
  after its first neutral final path appeared. The terminal retention contract
  worked: all build, sealed-stage, final-candidate, inventory, log, and failure-
  report evidence remains, and no post-final cleanup occurred.
- The sole repair is consumed. Sequence one is incomplete and sequence two was
  not started, so two complete passing unchanged sequences do not exist. No
  self-verifying manifest, seal authority, or
  `agent-ext20-rc13-builder-revalidation-v7-review.sh` may be created.
- V5, v6, and RC.12 remained exact before and after the attempted sequence.
  The v7 failure created no actual RC.13 builder, construction launcher,
  candidate/output/runtime path, ref, tag, workflow, artifact, release asset,
  suite, dogfood operation, release, external-use decision, or `EXT-20`
  completion. RC.13 remains unconsumed and `EXT-20` remains unchecked.
- No continuation is implied. Any independent failure review or fresh design
  identity requires new explicit operator direction and must treat RC.12, v5,
  v6, and v7 as immutable terminal history.

## RC.13 V6 Review Rejected; V7 Neutral Revalidation Authorized (2026-07-30)

- Independent review rejects the sealed v6 design for builder publication.
  Its full-role collision list requires tracked review/publication history to
  be absent, so an exact future builder would fail deterministically before
  construction. Full mode also lacks exact self-byte equality with sealed
  design authority, and stage/final root directories are outside its sealing
  and equality proof.
- V6 remains authentic sealed passing-neutral evidence but is not a publishable
  builder design. It cannot be repaired, rerun, copied, derived from, or reused.
  No RC.13 identity was consumed and no candidate or construction authority
  exists.
- The operator's next-gate direction separately authorizes one fresh v7
  neutral validation. It must use independently authored bytes and explicitly
  test full-role admission with permitted tracked history, exact builder and
  current-controller authority, root-inclusive sealed publication, and
  terminal retention after final-path appearance. Two complete sequences are
  mandatory.
- `agent-ext20-rc13-builder-revalidation-v7.sh` is the sole next gate after
  raw-Git publication. Raw Git published it with the v6 rejection as
  `4b90b5b511168034a890468a3336b71806c87300`; exact local, fetched, and public
  `main` plus its non-creating preflight passed. It cannot create a builder or
  candidate or grant construction, remote, suite, dogfood, release,
  external-use, or `EXT-20` authority.

## Prospective RC.13 V6 Design Is Accepted For Read-Only Review Only (2026-07-30)

- The sole authorized v6 prospective root is sealed persistent ignored
  `.revolvr/prospective-builder-validation-v6.bHfL29`, mode `0500`, with 13
  mode-`0444` files, 45,820 bytes, self-verifying manifest SHA-256
  `ecdd6f9f5a589038754d1bdb8326d5e19a1ea660eb0bb53a17029fa2aa7734be`,
  root stream
  `1b0c16fe2d886b60c04ea390b4d364bdfc9431dfde1617c1d34e7da28f8bc56f`,
  and inventory
  `0cd5ef032c89cf7be7a6872df665deb8d28481ee3beb80203473909cbdefbf41`.
- Both complete neutral sequences reached and passed every required gate with
  accepted bytes unchanged, no repair, and no residue. Trap cleanup identities
  are stable across success, induced failure, status-regression, early return,
  signal, and EXIT paths. The history baseline is exact at 1,662 bytes and 18
  lines with one final LF and no surplus newline. Every no-collision function
  has explicit success propagation.
- Rejected v5 remains immutable failed evidence at its exact 11-file,
  44,298-byte stream and inventory before and after each sequence. Passing v6
  does not repair, rehabilitate, relabel, derive from, or replace v5. RC.12 and
  all earlier candidates/designs remain terminal history.
- The fresh unexecuted design covers the complete construction procedure, but
  its neutral acceptance is evidence for independent review only. No full mode
  or product test/build ran, and no RC.13 builder, construction launcher,
  candidate, output path, ref, tag, workflow, artifact, suite, dogfood,
  release, external-use decision, or `EXT-20` completion exists.
- Inert tracked-path review launcher
  `agent-ext20-rc13-builder-revalidation-v6-review.sh` is the sole proposed
  next gate after independent controller inspection and raw-Git publication.
  Raw Git published the accepted record as
  `bb68b8016646e571bff1711f66dd81ff5ede5e7d`; exact local, fetched, and public
  `main` plus the launcher's non-executing preflight passed. It may only
  produce a read-only accept/reject recommendation and cannot execute retained
  design modes or create continuation. A favorable review still requires a
  new explicit operator decision before any builder publication or
  construction authority. `EXT-20` remains unchecked.

## Prospective RC.13 V5 Validation Is Rejected (2026-07-30)

- The sole authorized v5 prospective design root is
  `.revolvr/prospective-builder-validation-v5.tL50Wc`. It is unsealed rejected
  evidence, not a builder, candidate, construction design accepted for later
  publication, or review authority.
- Sequence one failed neutral cleanup because an EXIT trap outlived local probe
  variables. The one permitted repair removed the exact safely verified
  neutral residue and corrected both neutral cleanup lifetimes. The
  post-repair sequence then failed history preservation because the retained
  baseline contained one surplus terminal newline. No later repair is allowed.
- Passing syntax, semantic publication/cleanup, role/collision, focused static,
  exact-self-refusal, and forbidden-residue checks in the second sequence do
  not substitute for two complete passing sequences or for the unexecuted
  status-propagation regression. The design therefore cannot be sealed or
  reviewed.
- No RC.13 builder, construction launcher, candidate, construction/output
  path, ref, tag, workflow, Actions artifact, release asset, suite, dogfood,
  external-use decision, manifest, or review launcher exists. Independent
  controller verification accepts the v5 rejection and freezes its exact
  11-file, 44,298-byte stream and inventory as failed evidence.
- The operator's next-gate direction separately authorizes only a fresh v6
  anonymous prospective validation. It must independently author new bytes in
  a new persistent root, preserve v5 exactly, and make trap lifetime,
  canonical history EOF, no-collision status propagation, and full sequence
  completion explicit gates. `agent-ext20-rc13-builder-revalidation-v6.sh` is
  the sole next gate after raw-Git publication. Raw Git published it with the
  rejected-v5 record as `92eb3d85cad3e78f4e980da1031cca485c8ae8da`;
  exact local, fetched, and public `main` plus its non-creating preflight
  passed. It cannot create a builder or candidate or grant construction,
  remote, suite, release, external-use, or `EXT-20` authority. V5 must not be
  repaired, rerun, derived from, or reused; RC.12 remains terminal.

## RC.12 Failure Accepted; Prospective RC.13 Validation Authorized (2026-07-30)

- The completed independent review changed nothing. Controller replay accepts
  the immutable RC.12 failure record and deterministic pre-root
  status-propagation cause. RC.12 stays terminal; no historical builder,
  launcher, draft, output, or namespace may be retried, repaired, derived
  from, removed, relabeled, or reused.
- The operator's next-gate direction authorizes one fresh anonymous
  prospective RC.13 builder-design validation, not construction. It must use a
  unique persistent ignored root, independently authored bytes, two complete
  neutral validation sequences, and an explicit regression that every
  no-collision function returns success instead of propagating a false probe.
  At most one repair is allowed only between sequences.
- `agent-ext20-rc13-builder-validation.sh` is the sole next gate after raw-Git
  publication. Raw Git published it as
  `9e9e8740686efd17991da38b17fbda1d5eaaff0d`; exact local, fetched, and public
  `main` plus its non-creating preflight passed. It cannot run product
  tests/builds or full construction and cannot create an RC.13 builder,
  candidate, ref, workflow, artifact, suite, release, external-use authority,
  or `EXT-20` completion. Passing may retain only sealed prospective evidence
  and one inert later read-only review launcher.

## RC.12 Terminal Construction Status-Propagation Failure (2026-07-30)

- The operator's published no-argument construction gate executed the exact
  builder once. RC.12 is consumed and terminal despite stopping before any
  construction root or final path. Its builder and namespace cannot be
  retried, repaired, deleted, completed, relabeled, derived from, or reused.
- Exact builder lines 496-499 deterministically return failure when release
  assets are correctly absent: the final `grep ... && fail` AND-list has
  status `1`, the final loop and `verify_remote_collisions` propagate it, and
  `set -e` reaches the generic terminal trap. This happened before the first
  construction `mktemp`; all RC.12 construction/output identities remain
  absent. The failure is builder control flow, not product or environment
  evidence.
- `agent-ext20-rc12-construction-failure-review.sh` is the sole next gate after
  controller publication. Raw Git published the terminal record as
  `cfee541546da35ea60eac102996691f144279e4f`; exact local, fetched, and public
  `main` plus the launcher's non-executing preflight passed. The launcher may
  verify and independently review only this immutable terminal record; it
  cannot execute any RC.12 identity or create continuation. A future candidate
  requires a fresh collision-free identity, separate controller decision, and
  explicit operator authority. `EXT-20` remains unchecked.

## RC.12 Exact Builder And Construction Launcher Published (2026-07-30)

- Independent controller review accepted the exact builder publication and
  construction launcher without executing either. Raw Git published the
  tracked record as `b09a1c5d9973f39f2447711a58e03cacf8edf642`; exact local,
  fetched, and public `main` plus the construction launcher's non-executing
  preflight passed.
- `agent-ext20-rc12.sh` is now the sole local construction gate. Its
  no-argument execution is one-shot authority to run the exact reviewed
  builder. Any post-execution failure is terminal and forbids repair, retry,
  deletion, completion, or relabeling of RC.12.
- This gate grants no ref/workflow publication, remote CI, attestation, suite,
  dogfood/live model, tag, release, external use, recovery, queue, or `EXT-20`
  completion authority. RC.12 remains unconsumed until the builder begins.

## RC.12 Exact Builder Publication Prepared For Independent Review (2026-07-30)

- The accepted sealed draft was copied once to exact ignored builder
  `.revolvr/release-candidates/build-level1-v0.1.0-rc.12.sh`, mode `0555`,
  38,528 bytes, 756 lines, SHA-256
  `dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e`.
  It is byte-identical to the sealed draft and was not executed. The protected
  release-candidate parent was tightened from mode `0775` to `0755` before the
  builder appeared and must retain mode `0755`.
- Inert construction launcher `agent-ext20-rc12.sh` was fully authored and
  syntax/static-review validated only under a unique anonymous temporary root,
  then copied once to its tracked publication target. Its mode is `0755`, size
  14,242 bytes, line count 284, and SHA-256
  `f2a5f95323cf95334aed2c79c08368d63d0a73646600155a0032e6027bec6572`.
  It was not staged, committed, or executed. Its preflight cannot execute the
  builder; its no-argument path dynamically binds its own published hash and
  can exec only the exact builder after every authority/collision guard passes.
- Sealed root, manifest, stream, source/tree/product boundary, six controller
  anchors, nine historical controller hashes, and all RC.12 output/ref/tag/
  workflow/runtime/Actions/release-asset absences remained exact. These
  preservation results authorize no construction.
- Inert `agent-ext20-rc12-builder-publication-review.sh`, mode `0755`, 8,431
  bytes, 148 lines, SHA-256
  `3360511b57aeee4258e2d5530f5a4067202e9160eb524b58b735a2d3a4a70966`,
  is the sole next gate. It may produce only a fresh read-only accept/reject
  report and cannot execute construction or create further continuation.
  Favorable review still requires separate controller publication and new
  explicit builder-execution authority. RC.12 remains unconsumed and EXT-20
  remains unchecked.

## RC.12 Builder-Publication Gate Published (2026-07-30)

- Raw Git published the accepted fourth-design review and exact inert
  builder-publication gate as
  `2f21a4399a0a1bc00ceac345e0ebbeac9616d75a`. Exact local, fetched, and public
  `main` plus the complete clean non-creating preflight passed.
- `agent-ext20-rc12-builder-publication.sh` is now the immediate gate. It may
  create only the exact reviewed read-only builder, tracked inert construction
  launcher, and one later read-only publication-review launcher. It cannot
  execute construction or create candidate authority.

## Fourth Design Accepted For Exact Builder Publication (2026-07-30)

- The completed read-only review changed no repository or sealed-evidence byte.
  Independent controller review accepts the fourth design's manifest,
  sequence/repair record, surviving-history boundary, cleanup, role model,
  corrected collision/inventory coverage, and unexecuted terminal publication
  procedure.
- This acceptance grants only a separately reviewable exact builder and
  construction-launcher publication pass. It does not authorize executing the
  builder, construction, candidate creation, product tests/builds, remote
  publication, suite work, release, external use, or `EXT-20` completion.
- The exact builder must be byte-identical to sealed draft SHA-256
  `dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e`,
  mode `0555`, and immutable after appearance. The tracked construction
  launcher must be fully authored and syntax-checked anonymously before its
  exact path appears, verify all authority, expose a non-executing preflight,
  and dynamically bind its own final hash when later invoking the builder.
- `agent-ext20-rc12-builder-publication.sh` is the sole next gate after raw-Git
  publication. It cannot execute either published identity and may create only
  one inert review launcher on success. RC.12 remains unconsumed.

## Fourth Persistent Validation Record Published For Review (2026-07-30)

- Independent controller verification accepted the fourth sealed record after
  replaying its identities, histories, loss boundary, sequence/repair record,
  source/tool boundary, four corrected design concerns, and collision
  absences. Raw Git published the exact five-file scope as
  `bae8ff6b1e5d7e14a9002cd7fbba1ece101dc005`; exact local, fetched, and public
  `main` plus all named published review preflight diagnostics passed.
- The immediate gate is now the no-argument
  `agent-ext20-rc12-builder-revalidation-v4-review.sh`. It may inspect only the
  sealed persistent record and return a neutral accept/reject report. It must
  not execute draft modes, change files, or create any continuation.
- Favorable review alone still grants no builder publication, construction,
  candidate, remote, suite, release, external-use, or `EXT-20` authority.
  RC.12 remains unconsumed.

## Fourth Persistent Anonymous Design Accepted For Review (2026-07-30)

- The fourth independently authored anonymous prospective builder design
  passed sequence one, used its one permitted neutral repair, and passed the
  accepted sequence two with repaired bytes unchanged. Both accepted raw
  status tuples were `0,0,0,0,64,0,0`.
- Its sole reviewable authority is sealed persistent ignored root
  `/home/gernsback/source/revolvr/.revolvr/prospective-builder-revalidation-v4.5pWwTx`,
  mode `0500`, with 10 mode-`0444` files, 53,626 total bytes, verified
  nine-entry manifest SHA-256
  `f4cbe051d3b6cb13cb111b7374fb3d17c99e6f93979cb31853bbcd1db3da91c2`,
  and complete content stream
  `22e50f2dfc7ce0f0e935b080f567a0527b7e6a943234241161977c78fdaa9cd8`.
  The 38,528-byte, 756-line draft SHA-256 is
  `dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e`.
- Former `/tmp` validation roots and other missing construction roots remain
  terminal absences. Their durable summaries are requirements and historical
  claims, never replacement bytes or authority. Recovery must not recreate,
  reconstruct, or require those lost paths.
- The accepted review design covers the four controller defects across its
  initial design and sole neutral repair: post-seal inventory ordering, exact
  and descendant runtime collision coverage, every applicable RC.6-RC.11
  recorded absence, and all
  candidate/verification/attestation Actions artifact names. Its role model
  requires exact builder, tracked construction launcher, and sealed persistent
  root only in full mode; validation/recovery launchers are permitted history,
  while outputs and publication/runtime namespaces remain forbidden.
- The sole repair preserved execute bits for retained build instructions and
  binaries, made build-metadata tab matching exact, added still-applicable
  RC.6/RC.7 absence guards, and verifies copied final manifests. No repair was
  made after the accepted sequence two began.
- Neutral copy publication and cleanup are accepted for later inspection:
  writable construction, sealed two-level trees and mode-`0400` files, exact
  destination `mkdir`, only `cp -a source/. destination/.`, complete identity
  and distinct-inode checks, and exact depth-first cleanup after both success
  and induced failure. No probe residue survives.
- The full construction procedure is unexecuted design only. Its source,
  verification, tool/environment, vulnerability, reproducibility, metadata,
  manifest, post-seal inventory, and terminal `mkdir`/`cp -a` publication
  requirements grant no construction authority.
- Inert launcher `agent-ext20-rc12-builder-revalidation-v4-review.sh` is the
  sole later review gate after separate controller review/publication. It may
  return only a read-only accept/reject report, must not execute the draft, and
  cannot create continuation authority. Even favorable review requires a new
  explicit published builder/construction decision. RC.12 is unconsumed and
  `EXT-20` remains unchecked.

## RC.12 Persistent Fourth-Validation Gate Published (2026-07-30)

- Independent review and controller verification accepted the exact six-file
  volatile-root recovery preparation unchanged. Raw Git published it as
  `70aefe61ccdf6c6c6359558c483f6f1d9efac393`, and exact local, fetched, and
  public `main` plus the clean published v4 preflight passed.
- The immediate gate is now
  `agent-ext20-rc12-builder-revalidation-v4.sh`. It authorizes only one fresh
  independently authored fourth anonymous design and two-sequence neutral
  validation with persistent ignored evidence. It cannot create or execute a
  builder, construction launcher, candidate, full mode, product build, remote
  workflow, suite, release, or external-use action.
- Even a passing fourth neutral validation grants only a later inert
  independent review. RC.12 remains unconsumed and `EXT-20` remains unchecked.

## RC.12 Volatile Neutral-Evidence Loss Boundary (2026-07-30)

- Sealed root `/tmp/revolvr-builder-revalidation-v3.PKfbRl` disappeared before
  the published independent review. The review launcher failed at its exact
  root-presence guard before `codex exec`. The third draft's durable hashes and
  summaries prove the former record's identity claims but cannot substitute
  for its missing reviewable bytes. The third design is terminal lost evidence
  and grants no construction authority.
- Recovery must not recreate the old path, reconstruct draft bytes from old
  transcripts, or silently weaken review. RC.12 remains unconsumed because no
  builder, construction launcher, candidate identity, or full-mode execution
  occurred.
- A fourth independently authored anonymous design may be neutrally validated
  only after the recovery preparation is independently reviewed and published.
  Its sole retained sealed evidence must be created under persistent ignored
  `.revolvr/` state, not volatile `/tmp`. Semantic probes may use unique `/tmp`
  roots only when they are completely cleaned.
- The fourth design must treat lost roots as recorded absences and resolve
  post-seal verification-inventory ordering, exact RC.12 runtime-root
  collision coverage, complete retired runtime/ref/tag/glob absences, and all
  prior Actions artifact-name collisions before its first validation sequence.
  It retains the two-sequence neutral gate and cannot run product tests/builds
  or full construction.
- `agent-ext20-rc12-volatile-root-recovery-review.sh` is the sole immediate
  next gate. Passing it grants only later explicit publication authority for
  the recovery preparation. Builder publication, construction, candidate,
  remote, suite, release, external use, and `EXT-20` completion remain
  unauthorized.

## Third Anonymous Prospective Design Accepted For Review (2026-07-26)

- The independently authored third anonymous design passed two complete
  neutral validation sequences without using its repair allowance. Both
  sequences passed syntax, semantic admission, full-context role audit,
  focused static review, expected status-64 exact-self refusal, forbidden-
  identity/residue scans, and immutable-history checks.
- Neutral cleanup is now one exact-root guard. It rejects a non-active path,
  a non-directory or symlink-bearing tree, restores owner write permission to
  every directory depth-first, deletes only the proved tree depth-first, and
  proves absence. Cleanup status overrides an earlier probe status. Successful
  `mkdir`/`cp -a` publication and an induced pre-copy failure each proved
  cleanup through two sealed nested levels containing mode-`0400` files.
- The full-context design assigns roles instead of treating every prospective
  name as absent. Exact read-only builder, tracked construction launcher, and
  the sealed third validation root are required full-mode inputs; tracked
  validation-history launchers are permitted; candidate/verification outputs,
  post-candidate review, refs/tags/workflow/remote publication namespaces,
  and construction/runtime roots remain forbidden collisions.
- The unexecuted full design retains complete RC.6-RC.11/prior-root snapshot,
  checksum, and recorded-absence preservation; two exact non-local shallow
  source fetches excluding later controller objects; complete build and clean
  tool/environment instructions/evidence; both Go test/race/vet/module
  matrices; ordinary/verbose vulnerability results; reproducible target and
  embedded metadata; and terminal `mkdir` plus `cp -a` publication with
  complete stage/final identity checks. Full mode was not run.
- Sealed root `/tmp/revolvr-builder-revalidation-v3.PKfbRl`, draft SHA-256
  `302738277108837a7282d9abe5650d34bee5896ec58ef13fefa85b8edf9345fc`,
  evidence-manifest SHA-256
  `5018b79e47ea393eed9a3f0a3646f5d9ae20a65426993ed0bbe5c08a25b7e746`,
  and root stream
  `bf01b03207cbd2d3f056c31ee6c001f3efcff02a5fbace30cdeace228473bb77`
  are immutable prospective review evidence. They cannot themselves serve as
  a builder or candidate authority.
- The sole next gate is inert
  `agent-ext20-rc12-builder-revalidation-v3-review.sh`. It authorizes only a
  later read-only independent review of the sealed record. Even a favorable
  review requires a separately published exact builder and construction
  launcher plus new explicit operator authorization. No RC.12 identity was
  consumed, no construction/candidate/remote/dogfood/release/external-use
  authority exists, and `EXT-20` remains unchecked.
- Controller publication inspection found unresolved full-mode sequencing,
  exact-runtime-collision, recorded-absence, and remote-artifact coverage
  concerns. These do not invalidate the authentic neutral evidence, but they
  prohibit construction inference and are explicit required checks in the
  independent review.

## Third Anonymous Prospective Design Authorized (2026-07-26)

- Independent controller review accepts the second sealed revalidation root
  as authentic failed evidence and confirms the defect is confined to neutral
  probe cleanup. This does not rehabilitate or authorize the sealed draft.
- A third independently authored anonymous design is permitted because RC.12
  remains unconsumed and the prior failure occurred before any candidate
  identity, full mode, product build, or construction output.
- The third design may reuse independently verified requirements and exact
  historical constants, but no prior builder/draft bytes or implementation
  text. It must remeasure constants before validation, restore write access to
  every exact probe directory before depth-first deletion, exercise cleanup
  after both success and induced failure, and pass two complete neutral
  validation sequences.
- Passing that gate permits only a later inert independent-review launcher.
  Construction still requires a separately published exact builder and
  launcher plus explicit operator authorization. No candidate, remote,
  dogfood, release, external-use, or `EXT-20` authority follows from this
  decision.

## Prospective RC.12 Second Neutral Revalidation Rejected (2026-07-26)

- The second independently authored anonymous draft corrects the first
  draft's full-context contradiction and missing prospective construction
  requirements, but it is rejected because its second complete neutral
  validation sequence failed after the one permitted repair.
- Full-context authority is now modeled by role: the exact read-only builder
  and separately published construction launcher are required inputs; the two
  tracked validation-history launchers are permitted; and only candidate/
  verification outputs, the post-candidate local-review launcher, refs/tags/
  workflow/remote release and Actions namespaces, and runtime/suite/launch
  roots are forbidden collisions. Neutral full-context audit passed this
  distinction without creating an identity.
- The full design restores complete RC.6-RC.11 and two-neutral-root before/
  after preservation, terminal and staged manifest verification, non-local
  shallow exact-commit fetches excluding later controller objects, complete
  executable build instructions and source/tool/environment/command/
  vulnerability/reproducibility/artifact/inventory/history evidence, and
  terminal `mkdir` plus `cp -a` stage/final publication checks. Full mode was
  not run and these design improvements grant no construction authority.
- The first sequence's incorrect historical file counts were a neutral draft
  defect and consumed the sole repair. The second sequence then exposed a
  distinct cleanup defect: source/destination roots became writable, but
  their nested directories remained mode `0500`, preventing removal of the
  two mode-`0400` probe files. The exact probe was safely removed after
  inspection, and no residue or unrelated mutation remains.
- Sealed root `/tmp/revolvr-builder-revalidation.CSGs5E`, draft SHA-256
  `ae560b6eebc0c2d77721df426477727c473774cc1661db9dc9ef2194fd120768`,
  and evidence-manifest SHA-256
  `0b962f2cfc2095c530d4414676e17c53a06d8ef65223baf7d9f26e6e958453ee`
  are terminal failed neutral-validation evidence. They cannot be edited,
  copied, executed, derived from, published as a builder, or used as candidate
  input.
- No RC.12 identity was consumed and no builder, construction launcher,
  candidate, review launcher, remote authority, suite, live-model action,
  release, external-use decision, or `EXT-20` completion exists. A third
  anonymous design requires a fresh independent review and new explicit
  operator authorization; there is no executable continuation from this
  decision.

## Prospective RC.12 First Neutral Draft Rejected (2026-07-26)

- The first neutral-validation evidence is authentic and preserved, but its
  draft is not construction-ready. Passing syntax, semantic copy admission,
  and a neutral-path self refusal did not test full-mode runtime context.
- Full mode's admission is contradictory: exact self-identity requires the
  builder present, after which collision admission requires it absent. The
  same absence list incorrectly includes the required construction launcher
  and tracked validation-review launcher. This is a deterministic blocker,
  not an RC.12 candidate failure; no candidate identity has been consumed.
- Complete construction admission must distinguish required controller inputs
  from forbidden outputs. It verifies exact builder and construction launcher,
  permits immutable validation-history launchers, and rejects only actual
  candidate/ref/workflow/artifact/runtime/post-candidate-review collisions.
- A replacement neutral design must also restore complete RC.6-RC.11 before/
  after preservation, exact shallow source clones excluding later controller
  objects, and complete build instructions/tool/environment/vulnerability/
  target-metadata/bundle evidence from the settled reproducible procedure.
- The sealed first draft/root may be reviewed but never edited, copied,
  derived from, published as a builder, or used as candidate input. A second
  independently authored neutral draft must pass syntax, semantic admission,
  an explicit full-context audit, static review, and expected identity refusal
  before construction can be considered.
- Revalidation consumes no RC.12 identity and grants no construction, remote,
  suite, live-model, release, external-use, recovery, queue, or `EXT-20`
  completion authority.

## Prospective RC.12 Neutral Draft Accepted For Independent Review (2026-07-26)

- Neutral validation is a separate preconstruction authority boundary. The
  sealed anonymous draft at `/tmp/revolvr-builder-validation.maYqgv` passed
  syntax, semantic neutral admission, focused static audit, final syntax, and
  expected full-mode self-identity refusal. It creates no candidate identity
  and is not a builder publication or construction authority.
- The copy-publication contract is ordered write, file seal, nested/root seal,
  destination `mkdir`, `cp -a source/. destination/.`, destination-root seal,
  byte/mode/link/inode verification, parent write restoration, exact unique-
  root cleanup, and residue proof. Directory rename, hard links, and symlinks
  are excluded from the procedure.
- One neutral static-audit repair was appropriate after the first successful
  syntax/admission attempt: every Go metadata command now uses the same clean
  tool boundary as tests/builds, the executable version probe uses
  `--version`, all independent caches are explicit, and ERR/signal traps
  preserve correct failure status. Complete syntax/admission checks passed
  after the repair; no candidate namespace was consumed.
- The full-mode design is accepted only as prospective review material. It
  preserves source commit `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`
  and tree `2c8ee9f6b4283410547a9f99972e25eac06c9e33`, exact clean Go tools,
  independent clones/caches, both Go-version ordinary/race matrices, vet,
  module and vulnerability checks, reproducible supported amd64 artifacts,
  staged manifests, and `mkdir`/`cp -a` final publication with post-copy
  identity checks. It must not run until a later independent review and
  separate explicit construction authorization.
- Root `/tmp/revolvr-builder-validation.maYqgv` is immutable mode `0500`.
  Draft SHA-256 is
  `71b196d77b6eb89157492609b89e51d0c56a4e418b10b0f28ed43c94d5a4210d`;
  evidence-manifest SHA-256 is
  `2ae2f598dffd37f333c672f492438a81fb346c896964c0107d063423a515ae85`.
  Exact historical metadata and content inventories match before/after. RC.1
  through RC.11 and every recorded historical identity/absence remain
  immutable and cannot be inputs to a later builder or candidate.
- `agent-ext20-rc12-builder-validation-review.sh` is the sole next gate. It
  may start one fresh read-only independent review of the sealed draft and
  evidence, but it cannot execute either draft mode, construct or publish a
  builder/candidate, create a construction launcher, run tests/builds/live
  work, or grant remote, release, external-use, recovery, queue, or `EXT-20`
  completion authority. `EXT-20` stays unchecked.

## Prospective RC.12 Neutral Builder-Validation Boundary (2026-07-26)

- Independent review accepts RC.11 as terminal failed construction history.
  Its builder's neutral copy probe sealed the nested source directory before
  writing the probe file. The failure is a procedure-order defect, not a
  product, toolchain, copy mechanism, or filesystem-permission defect.
- Repeated candidate identities must not be consumed while validating builder
  mechanics. The next bounded task is therefore neutral builder validation,
  not RC.12 construction. It creates no RC.12-named runtime file or authority.
- A neutral prospective builder must expose `--neutral-admission` before its
  exact-self-identity boundary. That mode runs only read-only admission and a
  fully cleaned copy-publication probe: write under writable parents, seal,
  `mkdir`/`cp -a`, compare contents/modes, re-enable parent writes, and remove
  only the unique neutral probe.
- The neutral draft must pass `bash -n`, semantic admission, focused static
  audit, and an expected full-mode self-identity refusal. At most one neutral
  repair is allowed; any remaining failure stops without consuming RC.12.
  Construction authority requires later independent review and a separate
  launcher decision.
- This validation grants no builder publication, candidate, remote CI,
  attestation, suite, dogfood, live model, tag, release, external use,
  recovery, queue, or `EXT-20` completion authority.

## EXT-20 RC.11 Failed Copy-Probe Boundary (2026-07-26)

- RC.11 is terminal failed local-construction history. Its syntax-passing
  neutral draft and sole exact builder are byte-identical at 35,599 bytes,
  634 lines, and SHA-256
  `c92b7611028cf54abe37735c44fb116826193abd97673c1d69f8747f1b6f7355`;
  their modes are respectively `0444` and `0555`. Neither may be edited,
  executed, retried, repaired, completed, relabeled, deleted, mutated, derived
  from, or reused.
- The builder's one execution failed before preflight/build-root creation when
  its neutral copy-publication probe created the source child directory mode
  `0500` before attempting to create the probe file within it. The file write
  at line 295 was denied, the builder removed only its neutral probe as
  designed, and the explicit post-identity no-repair boundary exhausted
  RC.11. This is a builder procedure defect, not product, toolchain, test,
  artifact, manifest, or final-copy evidence.
- RC.11 produced no preflight, selected-tool record, build/stage root, clone,
  cache, diagnostic, artifact, candidate bundle, verification bundle, final
  path, suite, launch record, or local-review launcher. The intended final
  bundle paths remain absent and establish no local candidate authority.
- RC.6 and RC.7 terminal histories, RC.8 and RC.9 construction histories, and
  RC.10's sole malformed-builder identity remain exact at their recorded
  hashes, counts, modes, manifests, inventories, and absences. RC.11 grants no
  remote CI, attestation, dogfood, live model, suite, tag, release, external
  use, recovery, queue, or `EXT-20` completion authority.
- The next gate is independent controller review of the immutable RC.11
  failure record. A future candidate, if separately authorized, must use a
  fresh collision-free identity and independently authored neutral draft;
  this decision authorizes no executable continuation.

## EXT-20 RC.11 Draft-Before-Identity Boundary (2026-07-26)

- Independent controller review accepts RC.10 as terminal failed construction
  history. Its builder has one exact quoting defect: the final `file_hash`
  substitution on line 441 omits the inner closing quote. The builder remains
  immutable and unexecuted; no RC.10 candidate evidence exists.
- A candidate identity must not be consumed by an unparsed builder draft.
  RC.11 construction therefore authors only an anonymous neutral draft first,
  runs `bash -n`, permits at most one syntax repair while it remains neutral,
  and reviews command-substitution/inventory quoting before publishing any
  RC.11-named runtime file.
- Only syntax-passing draft bytes may be copied to the exact RC.11 builder.
  The exact builder is made read-only, required to match the draft hash, and
  parsed again before its sole execution. Once that named builder exists, any
  failure exhausts RC.11 and neither builder nor output may be repaired.
- Candidate source remains published commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`. Construction retains clean
  exact-Go invocation, independent builds/caches, full local verification,
  and `mkdir`/`cp -a` final publication with post-copy identity checks.
- RC.11 remains a local construction gate. It grants no ref/workflow
  publication, remote CI, attestation, suite, dogfood, live model, tag,
  release, external use, recovery, queue, or `EXT-20` completion authority.

## EXT-20 RC.10 Failed Builder Boundary (2026-07-26)

- RC.10 is terminal failed local-construction history. Its sole new ignored
  builder identity failed the first `bash -n` verification at line 443 before
  execution. Under the explicit no-repair rule, the builder is retained
  unchanged and RC.10 cannot be corrected, executed, retried, completed,
  relabeled, deleted, mutated, derived from, or reused.
- No RC.10 publication probe, selected-Go work invocation, preflight/build/
  stage root, clone, cache, diagnostic, artifact, candidate/verification
  bundle, final path, or local-review launcher exists. Thus RC.10 establishes
  no artifact, manifest, reproducibility, vulnerability, remote, suite, live,
  release, or external-use evidence.
- RC.6 and RC.7 terminal histories and RC.8 and RC.9 construction histories
  remain immutable at every previously recorded file, tree, stream, manifest,
  inventory, and absent-path identity. The syntax failure does not authorize
  repair or reuse of any historical material.
- `EXT-20` remains unchecked. The only next gate is an independent controller
  review of the exact RC.10 failure record. Any future candidate requires a
  new collision-free identity and separate explicit authority; no executable
  continuation or local candidate-review gate is created by this decision.

## EXT-20 RC.10 Copy-Publication Boundary (2026-07-26)

- RC.9 is accepted as a terminal environment/construction failure. Independent
  review reproduced that this execution environment denies moving a sealed
  directory from a staging parent into a sibling final path even though Unix
  ownership/modes, ACLs, ext4 mount state, and capacity permit it. This is not
  a product, artifact, or manifest defect.
- Neutral review proved that creating a destination directory and copying
  sealed contents with `cp -a source/. destination/` succeeds and preserves
  read-only modes. RC.10 must preflight that exact method and may not use
  directory rename, hard links, or symlinks for final bundle publication.
- The operator's next-task direction authorizes one fresh local construction
  pass for `level1-v0.1.0-rc.10`, sourced only from published commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`. RC.9 staged artifacts,
  binaries, clones, caches, builder, and results are immutable history, not
  RC.10 inputs.
- Final RC.10 paths are created only after independent builds and staged
  verification pass. Each is populated by archival copy and must match its
  stage in inventory, inventory hash, regular-file count, content-stream hash,
  and modes. Once either final path exists, any failure exhausts RC.10; no
  cleanup, completion, or repair is authorized.
- RC.10 remains a local construction gate. It grants no ref/workflow
  publication, remote CI, attestation, suite, dogfood, live model, tag,
  release, external use, recovery, queue, or `EXT-20` completion authority.

## EXT-20 RC.9 Failed Local-Construction Boundary (2026-07-26)

- The sole authorized `level1-v0.1.0-rc.9` local-construction attempt is
  terminal failed history. Exact source, toolchain, collision, preservation,
  regression, full-suite, vet, module, vulnerability, reproducibility,
  embedded-metadata, build-ID, and staged-manifest checks passed. Publication
  of the sealed staged candidate directory to its authorized final path was
  denied by the filesystem before either final candidate/verification path
  appeared.
- Preserve the exact read-only builder and diagnostic, unique preflight/build/
  staging roots, sealed staged subtrees, and absent final paths at the hashes
  recorded in `.agent/STATE.md` and `.agent/HANDOFF.md`. They are not a local
  candidate, bundle, review authority, or input to future construction. They
  cannot be retried, repaired, completed, reconciled, moved, relabeled,
  deleted, mutated, derived from, or reused.
- Passing staged evidence does not broaden authority after the final-path
  publication failure. RC.9 grants no ref/workflow publication, remote CI,
  attestation, suite, dogfood, Revolvr/model operation, tag, release,
  external-use, recovery, queue, local candidate review, or `EXT-20`
  completion authority.
- The next gate is an independent controller review of the immutable RC.9
  failure record. A future candidate, if explicitly authorized, must use a new
  collision-free identity, independently authored construction procedure, and
  unique roots; no executable continuation is authorized here. RC.6, RC.7,
  and RC.8 remain separately immutable at their recorded identities, and
  `EXT-20` remains unchecked.

## EXT-20 RC.9 Clean Construction Boundary (2026-07-26)

- Independent controller review accepts RC.8 as a terminal construction-
  environment failure and publishes only its durable failure record. The
  failed builder, diagnostic, partial roots, and absent final paths remain an
  immutable exhausted namespace; none may become RC.9 input.
- The operator's next-task direction authorizes one fresh local construction
  pass under collision-free identity `level1-v0.1.0-rc.9`. Candidate source
  remains exact published commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`; later controller records and
  launchers are not candidate source.
- Go-tool admission removes inherited `GOROOT`, `GOTOOLDIR`, and `GOFLAGS`,
  sets `GOENV=off` and `GOTOOLCHAIN=local`, and requires the selected
  executable's exact SHA-256/version to resolve the matching toolchain root
  and tool directory. Every construction Go invocation repeats this boundary
  and uses independent task-specific caches.
- RC.9 is local construction and verification only. It grants no ref/workflow
  publication, remote CI, attestation, suite, dogfood, live-model, tag,
  release, external-use, recovery, queue, or `EXT-20` completion authority.

## EXT-20 RC.8 Failed Local-Construction Boundary (2026-07-26)

- The sole authorized `level1-v0.1.0-rc.8` local-construction attempt is
  terminal failed diagnostic history. Exact source authority and all collision
  checks passed, but the Go 1.22.12 source-floor suite was invoked under an
  inherited `GOROOT=/usr/local/go` and selected Go 1.26.5 tools, producing a
  tool-version mismatch before any candidate artifact was built.
- Preserve the ignored builder, unique diagnostic, two exact source clones,
  caches, and unsealed partial verification tree exactly as retained. They are
  not a candidate, bundle, or authority and cannot be retried, repaired,
  completed, relabeled, overwritten, or reused. The absent final RC.8 bundle
  paths must not be populated under this exhausted pass.
- A candidate-construction environment must bind `GOROOT` to the selected
  exact toolchain or remove inherited `GOROOT` before invoking a different Go
  executable. Recording both executable identity and `go env GOROOT` is part
  of future source-floor admission. This construction rule changes no product
  source or dependency.
- No local-review launcher is appropriate without sealed candidate and
  verification bundles. Further work requires a separate independent
  controller review plus new explicit operator authority for a fresh
  collision-free candidate identity; nothing in the failed RC.8 attempt
  grants remote CI, attestation, suite, dogfood, live-model, tag, release,
  external-use, recovery, queue, or `EXT-20` completion authority.
- RC.6 and RC.7 remain independently immutable at all six recorded protected
  content-stream hashes, with both terminal checksum manifests passing before
  and after the failed RC.8 attempt. `EXT-20` remains unchecked.

## EXT-20 Published Planner Remediation And RC.8 Source Boundary (2026-07-26)

- The independently reviewed planner lifecycle-contract remediation is
  accepted and published as exact source commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`. This commit is the only source
  authority for a possible RC.8 local candidate.
- The later controller commit containing `agent-ext20-rc8.sh` and durable
  publication records is intentionally not candidate source. RC.8 construction
  must prove the pinned source is published and in current `main` ancestry and
  that `.agent/profiles`, `cmd`, `internal`, `go.mod`, and `go.sum` are
  unchanged from it.
- RC.8 local construction is a distinct no-live gate. It may reproduce and
  retain candidate/verification bundles but cannot publish refs or workflows,
  prepare or launch a suite, invoke Revolvr dogfood, tag, release, approve
  external use, grant recovery/queue authority, or complete `EXT-20`.
- RC.6 and RC.7 remain immutable terminal failed-attempt history. A fresh
  candidate number does not authorize reuse, derivation, repair, retry, or
  mutation of any historical launcher, suite, operation, record, or evidence.

## EXT-20 Planner Lifecycle Contract Independent Review (2026-07-26)

- Independent review accepts the current uncommitted planner lifecycle-
  contract remediation without a source or test correction. The checked-in
  profile/default template, generated planner prompt, Structured Outputs
  schema, Go validation, and planning-revision comparison now express one
  coherent lifecycle contract.
- Nested status-specific `anyOf` branches are within the repository-guarded
  Structured Outputs subset. Each branch is a closed object with all
  properties required, singleton status branches are distinct, and all
  Go-valid step and acceptance lifecycle dispositions are represented without
  weakening `PlanStep.Validate` or `AcceptanceCriterion.Validate`.
- Nil and empty evidence slices are equivalent only for planning-revision
  value comparison, matching schema-required `[]` with canonical state JSON
  that omits empty slices. Nonempty terminal evidence and every other stable
  step or criterion field remain exact.
- The review verification matrix and both pre/post RC.7 terminal-bundle hash
  checks passed. The accepted local result grants only a separate controller
  decision about committing and publishing the exact current working delta;
  it grants no publication itself and no RC.8, live, tag, release,
  external-use, queue, recovery, or `EXT-20` completion authority.

## EXT-20 Planner Lifecycle Contract (2026-07-26)

- Preserve `PlanStep.Validate` and `AcceptanceCriterion.Validate` as the
  authoritative fail-closed semantic boundary. RC.7 exposed a model-facing
  contract mismatch, not a reason to accept disposition evidence or rationale
  on nonterminal work.
- The repo-authored/default planner profile and the generated worker prompt
  must both state the lifecycle representation at the point of planning:
  pending/in-progress steps and pending criteria use empty evidence and null
  rationale. Evidence that grounds future work belongs in plan provenance or
  top-level inputs. Terminal evidence and skip/disposition rationale retain
  their existing meanings.
- The planner Structured Outputs schema encodes those rules with nested
  status-specific `anyOf` branches. This uses the already guarded supported
  subset, keeps all objects closed and all fields required, and prevents the
  known invalid representation before post-model parsing without weakening
  Go validation.
- JSON requires `evidence: []` for nonterminal planner output, while canonical
  Go state may omit an empty slice. Planning revision equality therefore uses
  value equality for evidence slices, under which nil and empty are equal.
  Nonempty terminal evidence, status, description, rationale, source, stable
  identity, and terminal ordering remain exact.
- This remediation is local and uncommitted pending a fresh independent
  review. It creates no RC.8 or live authority. The historical detail formerly
  accumulated in `.agent/HANDOFF.md` remains durable in this file,
  `.agent/STATE.md`, Git history, and immutable evidence; the handoff is now a
  concise active pointer.

## EXT-20 RC.7 Retired After Planner-Contract Failure (2026-07-25)

- RC.7 consumed its sole live authority and is terminally retired. Only
  operation `ext20-14b2bf40212b-01` ran; it stopped
  `unsafe_or_ambiguous` before implementation because the planner proposed
  `pending` steps containing evidence and rationale that the authoritative Go
  state validator reserves for terminal/skip dispositions.
- Both model calls completed successfully. The failure is a product contract
  mismatch between the planner profile/prompt, structurally permissive JSON
  schema, and stricter safe semantic validator—not an API outage. The validator
  remains the correct fail-closed boundary and must not be weakened merely to
  accept the live output.
- The next remediation must make the model-facing planner contract explicit:
  nonterminal steps use `evidence: []` and `rationale: null`; terminal step
  evidence and skip rationale retain their existing meanings. Confirm the
  smallest coherent prompt/schema approach from the full provenance path and
  add regression coverage before any fresh candidate decision.
- Preserve RC.7 launch record
  `ext20-14b2bf40212b-20260725T183415Z-1784416`, terminal evidence, and suite
  at recorded content-stream hashes. Never rerun, repair, delete, mutate,
  derive from, or reuse RC.7. RC.6 remains independently immutable. A future
  live attempt requires a fresh RC.8 suite and separate explicit authority.
- `EXT-20` remains unchecked. There is no active live command and no retry,
  recovery, tag, release, external-use, queue, or completion authority.

## EXT-20 RC.7 Authorized One-Shot Wrapper (2026-07-25)

- Human one-time live authority is durably separated at published commit
  `92d9c38ec9e4bbc01b0b5cf2c75ff3819f36658d`, tree
  `24e43c5eaf002c272aa27bca578345415427bc7e`. Executable wrapper
  `agent-ext20-rc7-live-once.sh`, SHA-256
  `240974ac774139e34a8f2f717d2c20e2eb01521d4d98c7bb25a43681d6888066`,
  is the sole admitted operator entry point.
- The wrapper requires one exact non-secret execution flag, clean published
  main, exact authorization/direct-launcher/backlog identities, and an absent
  launch-record root before delegating to the direct launcher's complete
  preflight and retained internal confirmation.
- A created launch-record root exhausts authority regardless of terminal
  result. No retry, recovery, merge, push, tag, release, external-use, queue,
  or `EXT-20` completion authority is implied. RC.6 remains immutable.

## EXT-20 RC.7 One-Time Human Live Authorization (2026-07-25)

- The operator explicitly responded `Authorized` to the immediately preceding
  consent request that enumerated exactly eleven RC.7 real-Codex operations,
  ten tasks, two disposable repositories, approval/sandbox bypass, inherited
  host environment/network, model cost/time, cancellation, commits, and
  immutable retained evidence. This grants one-time authority for that exact
  live suite scope.
- Authorization is fail-closed and non-retryable: existing launch or operation
  evidence ends the authority. It grants no recovery, merge, push, tag,
  release, external-use, queue, or `EXT-20` completion authority and no
  authority over RC.6.
- A controller may construct and publish one confirmation-gated wrapper after
  no-model verification, but may not execute it. The operator's later exact
  wrapper invocation is the separate execution action. Until publication and
  readback pass, no live command is active.

## EXT-20 RC.7 Await Explicit Human Live Authorization (2026-07-25)

- Independent controller replay accepted and raw Git published the exact human-
  authorization checkpoint record as commit
  `3c621ddfb8d11905150498c9cd3cc173323ac816`, tree
  `d5a313ed6a86d73bedb9ad6b1997f2e8e32e5b2`. Technical admission remains
  ready, but explicit human consent for real model effects is absent.
- Executable `agent-ext20-rc7-await-live-authorization.sh`, SHA-256
  `4a90526c5e5fc26c57470ac6cae197976aca449d7394899c4ca37b9efc23b332`,
  is the only safe continuation. It performs check-only authority readback and
  prints the consent boundary; it has no Codex, live-token, `--live`, launch-
  record, or mutation path.
- Running the inert status script is not live authorization. No further live
  construction or execution is authorized until a human explicitly consents
  to the recorded eleven-operation scope and material effects. `EXT-20`
  remains unchecked and no live command is active.
- RC.1 through RC.6 remain immutable rejected or failed history. RC.6 may be
  rehashed read-only but cannot be executed, repaired, deleted, derived from,
  or reused.

## EXT-20 RC.7 Explicit Human Consent Requirement (2026-07-25)

- The exact published admission recommendation and a fresh complete
  direct-launcher `--check` establish technical readiness only. They do not
  authorize the one-shot live suite, and running this no-model checkpoint does
  not imply consent. The current operator direction contains no explicit live
  authorization, so the gate stops here with `EXT-20` unchecked.
- Knowing consent must cover exactly eleven real-Codex operations over ten
  tasks in two disposable repositories: five ordinary successful changes,
  correction/final-verification/re-audit, needs input, intentional
  verification failure, cancellation/new-operation restart, and safety
  refusal. It must also cover sandbox/approval bypass, inherited host
  environment, unenforced isolation and network policy, real model cost and
  elapsed time, workspace/source/control-metadata commits, deliberate process
  interruption, and retained model/runtime/ledger/receipt/evidence output.
- A future decision, if any, is one-shot: launch/pre-start/status records and
  per-operation evidence are retained through terminal failure or
  interruption, while a pre-existing launch record or operation evidence
  prevents automatic retry. This checkpoint grants no recovery, merge, push,
  tag, release, external-use, or queue authority and leaves no live command
  active.
- RC.6 remains immutable failed history. It may be rehashed read-only but may
  not be executed, repaired, deleted, derived from, or reused.

## EXT-20 RC.7 Human-Authorization Checkpoint Boundary (2026-07-25)

- Independent controller replay accepted and raw Git published the final
  no-model admission recommendation as commit
  `1a07077f24526b9202da55c2981911b7e0770a67`, tree
  `1cb2ed843d1d5cf294487e8e404a030bb9f9838c`. Complete no-model replay and
  clean published `--check` readback passed; this remains technical readiness
  evidence, not human live authority.
- The next bounded authority is executable
  `agent-ext20-rc7-live-authorization-checkpoint.sh`, SHA-256
  `ca537cd5ef45afb23c31636c794653425e324ccfff2550532fbb0b7b83faa410`.
  It may only record the exact eleven-operation, ten-task, two-repository real-
  model effects for which explicit human authorization is absent.
- Running the checkpoint is not consent to a live launch. It cannot invoke
  `--live`, supply or expose the confirmation, create or edit an executable,
  create a launch record, start Revolvr or a nested model, complete `EXT-20`,
  or grant tag, release, external-use, or queue authority. No live command is
  active.
- RC.1 through RC.6 remain immutable rejected or failed history. RC.6 may be
  rehashed read-only but cannot be executed, repaired, deleted, derived from,
  or reused.

## EXT-20 RC.7 Final No-Model Admission Recommendation (2026-07-25)

- The final no-model admission review passed against clean, freshly fetched
  local/fetched/raw-Git/public-REST `main` at
  `b6351d108fc971dcfff5367267fb7eb1a3b00273`. Published pre-live record
  `f03258496621bbf8fd440a5c7293430a6ce44a22`, tree
  `39ac014ebdcd56482b931fd79073429e51419df3`, parent
  `460c2fa31155dca28a4f9ce861c03fbad8949acc` and its three-file durable-state
  delta are exact in both required ancestries.
- Exact launcher/checker inspection, syntax, independent safe argument
  refusals, complete published `--check`, raw-Git/public-REST authority, both
  sealed candidate bundles, candidate/Codex/script identities, prepared suite,
  plan, repository/task/doctor/source-writer/sentinel authority, full Go tests,
  and diff hygiene passed. Before and after there was no operation, collector,
  model, receipt, aggregate, or RC.7 launch-record evidence, and all ten tasks
  remained pending and zero-attempt.
- Recommendation is **PASS for a separate later human/controller live-
  authorization decision**. This recommendation is not live authority: it
  does not authorize an invocation, launch record, Revolvr/model operation,
  tag, release, external use, queue use, or `EXT-20` completion. No live
  command is active.
- RC.1 through RC.6 and every historical authority/evidence root remain
  immutable rejected or failed history. The protected RC.6 suite, launch
  record, and terminal evidence remain exact and may not be executed,
  repaired, deleted, derived from, or reused.

## EXT-20 RC.7 Final No-Model Live-Authority Review Gate (2026-07-25)

- Independent controller replay accepted and raw Git published the exact
  pre-live review record as commit
  `f03258496621bbf8fd440a5c7293430a6ce44a22`, tree
  `39ac014ebdcd56482b931fd79073429e51419df3`. Complete no-model replay and
  clean published `--check` readback passed; RC.7 remains unstarted, RC.6
  remains preserved, and `EXT-20` remains unchecked.
- The next bounded authority is executable
  `agent-ext20-rc7-live-authority-review.sh`, SHA-256
  `79d8aea3ec9f46d4ccee8bdcdd33aec09502e4d0e7ed65acef2b54dbd93e563f`.
  It may only review exact published/runtime authority and record a durable
  pass/fail recommendation for a later human/controller decision.
- This gate cannot invoke or activate the live path, supply or encode the live
  confirmation, create a launch record, start Revolvr or a nested model,
  construct another executable, complete `EXT-20`, or grant tag, release,
  external-use, or queue authority. No live command is active.
- RC.1 through RC.6 remain immutable rejected or failed history. RC.6 may be
  rehashed read-only but cannot be executed, repaired, deleted, derived from,
  or reused.

## EXT-20 RC.7 Independent Pre-Live Review Boundary (2026-07-25)

- Independent pre-live review accepted the complete published direct launcher
  and focused checker at exact SHA-256
  `9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45`
  and `c434380918c8ec726504126ac42b3a1338fa34741329ac60b28d44c03fc4f414`.
  Clean local, fetched, and public `main` agreed at
  `460c2fa31155dca28a4f9ce861c03fbad8949acc`; published direct-launch commit
  `d2e7d8ddda66db7685b021dcc33b94766ec00204`, tree
  `701639e624fc86a0a18c77cf19dc064f8e7bd511`, parent
  `6931b3ef790a6f0375944043596cb591cf589f2a`, remained exact in local and
  fetched ancestry.
- Syntax, independent missing/wrong/multiple argument refusals, the complete
  published `--check` preflight, `go test -count=1 ./...`, and diff hygiene
  passed. The prepared RC.7 suite remained exact with zero operation,
  collector, model, receipt, aggregate, or launch-record evidence and ten
  pending zero-attempt doctor-ready tasks. All three protected RC.6 streams
  remained exact at their recorded hashes.
- This result grants only authority for a separate controller to review and,
  if accepted, publish the exact three durable-state files. It does not make a
  live command active, authorize a live invocation or launch record, start a
  Revolvr/Codex/model operation, complete `EXT-20`, or grant tag, release,
  external-use, or queue authority. No executable continuation was created;
  any later live decision must be a separate post-publication authority gate.
- RC.1 through RC.6 and all historical refs, workflows, bundles, artifacts,
  suites, operations, launch records, diagnostics, and evidence remain
  immutable rejected or failed history. RC.6 cannot be executed, repaired,
  deleted, derived from, or reused.

## EXT-20 RC.7 Published Direct Launcher Pre-Live Review Gate (2026-07-25)

- Independent review accepted and raw Git published the exact RC.7 direct
  launcher/checker construction as commit
  `d2e7d8ddda66db7685b021dcc33b94766ec00204`, tree
  `701639e624fc86a0a18c77cf19dc064f8e7bd511`. Clean local, fetched, and
  public `main` readback and the complete published `--check` preflight passed
  without a model call, launch record, or runtime mutation.
- Publication does not authorize the live path. The next bounded authority is
  executable `agent-ext20-rc7-prelive-review.sh`, SHA-256
  `f013427c57678950a17fc71a71e23a89e44fac30668d12d47971d6475c551b12`.
  It may perform only a fresh no-model review of the exact published launcher,
  all remote/bundle/suite/task and preservation authority, and the check-only
  path; it may update only the three durable-state files.
- The pre-live review may not supply or reproduce the live confirmation,
  invoke `--live`, create a launch record, start Revolvr or a nested model,
  alter the launcher/checker, complete `EXT-20`, or grant tag, release,
  external-use, or queue authority. Its result must return for separate
  controller review, and no live command is active.
- RC.6 remains terminally retired and immutable at its recorded protected
  hashes. The pre-live review may verify those streams only through read-only
  hashing; no RC.6 launcher or suite path may be executed or reused.

## EXT-20 RC.7 Direct Launcher Review Authority (2026-07-25)

- The only newly constructed RC.7 direct-gate implementation is executable
  `agent-ext20-rc7-live-direct.sh`, SHA-256
  `9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45`,
  with focused checker `scripts/check-ext20-rc7-live-direct.sh`, SHA-256
  `c434380918c8ec726504126ac42b3a1338fa34741329ac60b28d44c03fc4f414`.
  These unpublished bytes are review inputs, not live authority.
- The launcher fails before authority checks unless given exactly `--check` or
  the retained live confirmation. Its complete preflight binds published
  main, every RC ref and absent RC tag, the exact source/attestation/companion
  runs and jobs, the sole attestation artifact, both sealed candidate bundles,
  script identities, the prepared suite and plan, candidate/Codex identities,
  repositories, ten task states and doctor results, source-writer authority,
  sentinels, zero evidence/aggregate, absent launch-record root, unchanged
  task backlog, and all three protected RC.6 streams.
- Check mode has no mutation or model path. A later separately authorized live
  invocation may create exactly one previously absent ignored RC.7
  launch-record root only after the complete preflight, retain exact pre-start
  authority and terminal/interruption status, and start the guarded suite once.
  Any existing RC.7 launch-record root or operation evidence refuses the
  launcher; there is no retry, recovery, or evidence-reuse path.
- Construction checks passed without the live token: syntax, focused refusal
  and collision checks, direct downstream read-only preflight, full Go tests,
  diff hygiene, both sealed RC.7 bundles, raw-Git/public-REST evidence, exact
  prepared content, and RC.6 preservation. The next authority is fresh
  independent no-model review and controller publication of the exact
  five-file delta. No active live command, model operation, launch record,
  tag, release, external-use approval, queue authority, or `EXT-20` completion
  is granted.

## EXT-20 RC.7 Direct-Live-Gate Construction Authority (2026-07-25)

- Independent controller review accepted and raw Git published the exact RC.7
  prepared-suite record as commit
  `83078e8467f00439956252955d5c130d51f34214`, tree
  `31e680996695e1bc71d38e1216a471250009fb0d`. The ignored suite root remains
  exact, unstarted, and free of evidence, aggregate output, and launch records.
- The next bounded authority is construction and no-model checking only via
  `agent-ext20-rc7-live-gate.sh`. It may add one fail-closed RC.7 direct
  launcher and its focused checker, bound to the published suite, candidate,
  attestation, script, Codex, repository, and preservation identities.
- Construction may exercise only refusal and `--check` paths. It cannot invoke
  `--live`, pass a live confirmation, create a launch record, start Revolvr or
  a nested model operation, or publish an active live command. The resulting
  files require a fresh independent controller review before any later live
  authorization decision.
- RC.6 remains terminally retired and immutable. No RC.6 launcher or suite
  path may be executed, repaired, deleted, mutated, derived from, or reused.

## EXT-20 RC.7 Prepared Suite Authority And Review Gate (2026-07-25)

- The guarded suite's sole admitted candidate authority is now exact RC.7
  source `f63cbe3989cb281652cf4eec3f92614fec98294d`, Linux artifact SHA-256
  `1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a`,
  and bundle `.revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb`.
  Every non-candidate byte of the suite driver remains unchanged.
- The sole newly prepared RC.7 suite authority is persistent root
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite`, suite ID
  `ext20-14b2bf40212b`, authority/plan/content-stream SHA-256 values
  `c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e`,
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`,
  and `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`.
  Its repository heads are
  `22bc5fd5ea1469fb76afef6425964f0b0c7f70bb` and
  `f92954597d8bd35372ee181c959be9a5fc637429`.
- Preparation and independent inspection started no model and retained zero
  operation/collector manifests, an empty aggregate, ten pending doctor-ready
  tasks, intact sentinels, exact candidate/Codex/configuration authority, and
  no launch record. `--verify-suite` is post-live evidence verification and
  correctly refuses this unstarted suite at its first missing manifest; that
  expected refusal is not authority to manufacture evidence or change the
  verifier.
- This prepared root is not published or live authority. A fresh independent
  no-model controller must review the exact four-file tracked delta and every
  retained authority/preservation claim before publication. No live command
  is the active handoff, and this decision grants no model operation, launch
  record or launcher, tag, release, external-use approval, queue authority, or
  `EXT-20` completion. RC.1 through RC.6 remain immutable rejected or failed
  history.

## EXT-20 RC.7 Fresh Suite-Preparation Gate Authority (2026-07-25)

- Independent controller review accepted and raw Git published the exact RC.7
  remote-attestation record as commit
  `c17a8f08efe1fedb1edcdc5f98b6d03ebc0e5a3c`, tree
  `5d8580dc2b7a847c8d4ec83791781d3568b23296`. Candidate/attestation refs,
  dedicated run/job/artifact, companion CI, sealed bundles, retained local
  replay, and historical preservation are exact.
- The next admitted bounded action is launcher `agent-ext20-rc7-suite.sh`.
  It may update only the guarded suite candidate source, Linux hash, and bundle
  path from RC.6 to exact RC.7 authority and prepare exactly one new persistent
  collision-free `.revolvr/ext20-rc7.XXXXXX/suite` root.
- The prepared RC.7 suite must be newly created and independently verified; it
  cannot reuse, derive from, inspect with current mutable suite tooling, or
  mutate the retired RC.6 suite, launch record, or terminal evidence. RC.6
  remains immutable failed history.
- Suite preparation grants no `--live` invocation, model operation, launch
  record, live launcher, tag, release, external-use approval, queue authority,
  or `EXT-20` completion. Its tracked delta and ignored prepared root require a
  fresh controller review before any later live-gate decision.

## EXT-20 RC.7 Remote Artifact-Attestation Authority (2026-07-25)

- Empty-expected raw Git created only
  `refs/heads/level1-v0.1.0-rc.7-attestation` at exact reviewed workflow commit
  `3cc6d527f889c7b933828fbd832d07b5291aee79`; exact remote readback agrees.
  Candidate source/ref remains
  `f63cbe3989cb281652cf4eec3f92614fec98294d`.
- Dedicated push run/job `30163857880` / `89693466274` completed successfully
  at the exact workflow head. Its sole unexpired artifact is `8621008768`,
  `level1-v0.1.0-rc.7-attestation`, `70275600` bytes, digest
  `sha256:ae87472ef86b5d25cca5df333f057f10d77cf74cd7f332f30d6770745bbf6356`.
  No read-only token was configured; the unauthenticated archive endpoint
  returned HTTP 401, so remote archive bytes are not claimed by the
  controller.
- Companion CI run `30163853353` completed successfully at the exact head with
  exactly its ten mandatory jobs: `89693455164`, `89693455158`, `89693455167`,
  `89693455176`, `89693455194`, `89693455193`, `89693455190`, `89693455186`,
  `89693455183`, and `89693455189`. Exact names, URLs, timestamps, head SHAs,
  and conclusions are retained in `.agent/STATE.md` and `.agent/HANDOFF.md`.
- Complete RC.7 bundle/workflow/local-replay, historical-ref/tag, and RC.6
  terminal-evidence checks passed before and after publication. RC.1 through
  RC.6 remain immutable rejected or failed history. `.agent/TASKS.md` remains
  unchanged with `EXT-20` unchecked.
- This authority stops at remote ref/run/job/artifact and companion-CI
  evidence. A fresh independent no-model controller must accept and publish
  the exact three-file record before a separate RC.7 suite-preparation gate
  may be authored or reviewed. No suite, Revolvr or model operation, tag,
  release, external-use approval, queue authority, or `EXT-20` completion is
  granted.

## EXT-20 RC.7 Remote Attestation Gate Authority (2026-07-25)

- Independent review accepted and raw Git published the exact RC.7 local
  attestation workflow/state scope as commit
  `3cc6d527f889c7b933828fbd832d07b5291aee79`, tree
  `1a35d15e75ce372d02f9499bd9634ca0f808f68d`. The admitted workflow remains
  `.github/workflows/level1-rc7-candidate-attestation.yml` at SHA-256
  `f3e06992e72029d80162c9b5901c398dbfb3c79cfeae43c0e72ddd28cff4ee13`.
- The next external-mutation authority is deliberately singular: launcher
  `agent-ext20-rc7-attestation-remote.sh` may create only absent ref
  `refs/heads/level1-v0.1.0-rc.7-attestation` at the exact reviewed workflow
  commit using an empty-expected lease. Any ref, tag, workflow, artifact, main,
  candidate, bundle, task, or protected-evidence drift fails the gate closed.
- After exact ref readback, the gate may only wait finitely for and record the
  dedicated RC.7 attestation run/job/artifact and companion CI run/jobs. It
  grants no suite preparation or reuse, Revolvr or nested model operation,
  tag, release, external-use approval, queue authority, or `EXT-20` completion.
  RC.6 remains terminally retired and immutable.

## EXT-20 RC.7 Local Artifact-Attestation Workflow Authority (2026-07-25)

- The sole locally admitted RC.7 attestation implementation is separate
  workflow `.github/workflows/level1-rc7-candidate-attestation.yml`, SHA-256
  `f3e06992e72029d80162c9b5901c398dbfb3c79cfeae43c0e72ddd28cff4ee13`,
  triggered only by a push to `level1-v0.1.0-rc.7-attestation`. Trigger HEAD is
  workflow authority only; release source remains exact candidate commit
  `f63cbe3989cb281652cf4eec3f92614fec98294d`, tree
  `43fc099d966cd6c06a74f00272c945fe3ca0a0f9`, with source-CI run
  `30160277511` recorded in the retained authority manifest.
- Exact Go 1.26.5, two independent clean non-local clones with distinct build
  and module caches, and the settled release flags must produce byte-identical
  Linux/Darwin/FreeBSD amd64 pairs with sealed hashes
  `1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a`,
  `fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086`,
  and `13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe`.
  Every retained binary requires exact Go/tool/path/compiler/trimpath/target/
  CGO/source/clean-VCS metadata, an empty build ID, and exact
  `main.version=0.1.0` authority.
- The single future artifact authority name is
  `level1-v0.1.0-rc.7-attestation`. The workflow fails closed unless its exact
  workflow/ref identity and candidate ref agree and the RC.7 tag, attestation-
  ref, attestation-namespace, and artifact-name authorities have no foreign
  collision. Before later publication, the exact candidate ref is the only
  admitted RC.7 ref.
- Complete unmodified embedded-shell execution from a fresh detached exact-
  source clone passed. Retained local root
  `/tmp/revolvr-ext20-rc7-attestation.Uq3syS` has exactly 29 attestation files
  with path-bearing content-stream SHA-256
  `5c56018487239868a8a5af83b14b37179b81e307d09956650085042be8ad31e0`.
  Both RC.7 sealed bundles, exact candidate/CI authority, historical roots/
  refs/workflows/tags, and RC.6 terminal evidence reverified before and after.
- Local construction grants no commit, push, attestation ref, remote run or
  artifact, suite, model call, tag, release, external-use approval, queue
  authority, or `EXT-20` completion. A fresh independent reviewer must accept
  the exact four-file scope and retained replay before any separately
  authorized controller publication. Remote attestation publication remains a
  later independent gate; RC.1 through RC.6 remain immutable rejected/failed
  history.

## EXT-20 RC.7 Remote-CI Review And Attestation Gate (2026-07-25)

- Independent raw-Git/public-REST review accepted exact candidate ref
  `f63cbe3989cb281652cf4eec3f92614fec98294d` and successful ten-job CI run
  `30160277511`. Both sealed RC.7 bundles and RC.6 preservation remained exact.
  The accepted record is published as controller commit
  `a0f5b069ea5189b11f1d4de71c93e825814adee5`.
- The next bounded authority is local-only creation and full exercise of
  `.github/workflows/level1-rc7-candidate-attestation.yml`. The workflow must
  check out the exact candidate source and reproduce the sealed artifacts;
  trigger HEAD is workflow authority only.
- This grants no workflow commit/push, attestation ref, remote run/artifact,
  suite, live model, tag, release, external-use, queue, or EXT-20 completion
  authority. RC.6 remains immutable failed-attempt evidence.

## EXT-20 RC.7 Exact-Candidate Remote CI Authority (2026-07-25)

- The RC.7 remote candidate authority is exact branch
  `refs/heads/level1-v0.1.0-rc.7` at source
  `f63cbe3989cb281652cf4eec3f92614fec98294d`, tree
  `43fc099d966cd6c06a74f00272c945fe3ca0a0f9`. Raw Git created it only after
  a no-tags fetch, exact local/remote-main agreement at
  `4bd712502a0a5fd27b80587b1ae38c916d92d390`, ancestry of published review
  record `1055d7fc8ccdb844b5fe1405674133a244a1be64`, complete RC.7 sealed-bundle
  verification, exact source reachability, and absence of all candidate,
  attestation, tag, workflow, and artifact-name collisions. The required
  empty-expected lease succeeded and exact remote readback agrees; no existing
  ref was moved or deleted and no local branch was created.
- Push-triggered CI run `30160277511`, run number `80`, attempt `1`, event
  `push`, branch `level1-v0.1.0-rc.7`, and exact head SHA
  `f63cbe3989cb281652cf4eec3f92614fec98294d` completed with conclusion
  `success`. Its URL is
  `https://github.com/ponchione/revolvr/actions/runs/30160277511`.
  Public REST returned exactly the ten required unique successful jobs:
  `89684369461` Go 1.22 source floor and tests, `89684369485` Production
  autonomous strict-fake suite, `89684369426` Race tests, `89684369423` Vet
  and module verification, `89684369490` Fake-Codex success smoke,
  `89684369458` Fake-Codex verification-failure smoke, `89684369418` Build
  linux/amd64, `89684369432` Build darwin/amd64, `89684369443` Build
  freebsd/amd64, and `89684369447` Build Windows diagnostic stub. Exact job
  URLs, times, head SHA, status, and conclusions are retained in
  `.agent/STATE.md` and `.agent/HANDOFF.md`.
- Post-CI preservation keeps both RC.7 inventory/seal pairs exact, every
  RC.1-through-RC.6 remote ref unchanged, and all historical and RC.7 tags
  absent. The RC.6 terminal manifest verifies and its suite, launch-record,
  and terminal-evidence content-stream SHA-256 values remain
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- The next separately bounded gate is local-only construction and full
  exercise of a collision-free RC.7 artifact-attestation workflow. It must use
  an exact detached checkout of candidate source
  `f63cbe3989cb281652cf4eec3f92614fec98294d`, exact Go 1.26.5, and reproduce
  the three sealed artifact hashes; trigger HEAD is workflow authority only.
  This decision grants no workflow commit/push, attestation ref, remote run or
  artifact, suite, live/model operation, tag, release, external-use, queue, or
  `EXT-20` completion authority.

## EXT-20 RC.7 Local Review And Remote-CI Handoff (2026-07-25)

- Independent controller review accepted the complete sealed RC.7 candidate
  and verification evidence from exact remediation source
  `f63cbe3989cb281652cf4eec3f92614fec98294d`. Raw Git published the reviewed
  three-file record as commit `1055d7fc8ccdb844b5fe1405674133a244a1be64`.
- The next bounded authority is empty-expected creation of only
  `refs/heads/level1-v0.1.0-rc.7` at that exact source followed by required
  push-triggered CI readback. This grants no attestation, suite, live model,
  tag, release, external-use, queue, or EXT-20 completion authority.
- RC.6 remains immutable failed-attempt evidence. Its suite, launch record,
  terminal evidence, and all historical candidate material must remain
  unchanged before and after the RC.7 remote gate.

## EXT-20 RC.7 Local Candidate Authority (2026-07-25)

- The sole local RC.7 candidate authority is exact published remediation
  source `f63cbe3989cb281652cf4eec3f92614fec98294d`, tree
  `43fc099d966cd6c06a74f00272c945fe3ca0a0f9`, release version `0.1.0`,
  source floor Go `1.22.12`, and release Go `1.26.5`. Later controller commit
  `6c932653f9ea79892569c71aa4e05e69947eaaa5` and
  `agent-ext20-rc7.sh` are not candidate source and are absent from candidate
  clones and artifacts. No product-source delta follows the remediation.
- The immutable candidate bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb`; its
  inventory/seal values are
  `7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba` /
  `2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0`.
  Linux, Darwin, and FreeBSD amd64 artifact SHA-256 values are
  `1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a`,
  `fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086`,
  and `13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe`.
  Two independent fresh non-local clean clones produced byte-identical pairs
  with module-readonly/local-toolchain mode, disabled CGO, amd64, trimpath,
  explicit clean VCS metadata, empty build IDs, and exact embedded version.
- The separate immutable verification bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb-verification`;
  its inventory/seal values are
  `ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733` /
  `6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b`.
  It retains exact source/tool/build/version/target identities, commands,
  focused ordinary/race regressions, Structured Outputs and lifecycle guards,
  production happy-path and strict-fake proofs, both full Go suites, vet,
  module verification, vulnerability output, artifact metadata, complete file
  inventories, collision evidence, and historical preservation.
- RC.1 through RC.6 remain immutable rejected or failed history. The RC.6
  terminal manifest and all sealed historical bundles reverified; the RC.6
  protected suite, launch record, and terminal evidence retained exact pre/post
  content-stream hashes
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- Local construction grants no candidate-ref publication, remote CI,
  attestation, dogfood, live/model operation, suite preparation, tag, release,
  external-use, queue, or `EXT-20` completion authority. The next bounded gate
  is fresh independent read-only review of the exact two bundles, the
  three-file tracked state scope, and preservation evidence before any later
  controller publication.

## EXT-20 RC.7 Next No-Model Gate (2026-07-25)

- After publication of the planner-profile remediation, the next bounded gate
  is fresh local RC.7 candidate construction from exact source
  `f63cbe3989cb281652cf4eec3f92614fec98294d`. The attended launcher may create
  and verify new local candidate and verification bundles only.
- RC.7 must not reuse an RC.6 candidate, bundle, suite, operation, launch
  record, or evidence. Local construction grants no remote publication,
  attestation, suite preparation, live model call, tag, release, external-use,
  queue, or EXT-20 completion authority.

## EXT-20 Planner Profile Identity And RC.6 Retirement (2026-07-25)

- A repo-authored run profile may be represented by either its exact protected
  file bytes or the exact surrounding-whitespace-normalized bytes returned by
  `prompt.LoadRunProfile`. Planner application now matches auditor application:
  read the protected file under a fixed 1 MiB bound, then require the recorded
  SHA-256 and byte size to match exactly one of those two representations.
  This admits ordinary final newlines without weakening non-whitespace tamper
  detection or protected-file containment.
- RC.6 is terminally retired after operation `ext20-7b4a5932090f-01` stopped
  `unsafe_or_ambiguous` at planner application. Its retained suite, launch
  record, and evidence are immutable failed-attempt evidence. No RC.6 launcher
  or suite command is future authority, and RC.7 must be constructed from a
  fresh candidate and later a fresh suite.
- This repair does not complete `EXT-20`, authorize a live run, reuse evidence,
  grant external use or queue authority, or create a tag or release.

## EXT-20 RC.6 Prepared-Suite Review And Direct Live Gate (2026-07-25)

- Independent controller review accepted the suite script's exact three-
  constant RC.6 specialization and retained persistent suite
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite`. Static and
  collector verification, `go test -count=1 ./...`, both sealed RC.6 bundles,
  all remote authority, exact prepared content, ten doctor-ready tasks, clean
  repositories, sentinels, zero evidence/aggregate, and all immutable RC.5
  streams passed without a model call or retained mutation.
- Raw Git published the exact reviewed four-file scope as commit
  `87be94f8fc4f04cb25c40598cc1f44cfe3b57efe`, tree
  `0e9009b53749d9f43706f57617bea04f8f84128e`. Suite script SHA-256 is
  `d16caafba9fb5fb8db83188d87006e57c8b77d88159d28502bb736e93672a3d7`;
  collector SHA-256 remains
  `2aa507930a12f4040fc8e1e359968b67d2be9cfa6e92aa65d9c8ce0577959cdd`.
- The only admitted live path is new direct foreground launcher
  `agent-ext20-rc6-live-direct.sh` with exact confirmation
  `EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. It fails closed on any authority drift,
  requires the still-unstarted suite, and writes pre-start authority, stdout,
  stderr, and durable exit/interruption status outside the suite beneath
  ignored `.revolvr/ext20-rc6-launch-records` before one guarded start.
- New `scripts/check-ext20-rc6-live-direct.sh` exercises only no-token refusal,
  unpublished-tree/check behavior, and isolated launch-record collision. It
  must preserve the suite and leave no RC.6 launch record. Preparing and
  checking the gate grants no model call, retry, tag, release, external-use,
  queue, or `EXT-20` completion authority.

## EXT-20 RC.6 Prepared-Suite Authority And Review Gate (2026-07-25)

- The guarded external Level-1 suite now derives authority only from immutable
  RC.6 source `73f1f81f1c51d927114f19818a18161d0fcb8541`, Linux artifact SHA-256
  `f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d`,
  and bundle `.revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51`.
  Every non-candidate suite authority remains unchanged, including exact Codex
  `0.144.4`, model, reasoning effort, plan, schemas, scenarios, thresholds,
  configuration, live confirmation, and collector behavior.
- The sole prepared RC.6 authority is persistent suite
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite`, suite ID
  `ext20-7b4a5932090f`, authority SHA-256
  `5d90db1fca978451f0fa9b0950bc71dd9b334a00e0647d7e16158698a2358e40`,
  plan SHA-256
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`,
  and relative-path-bearing content-stream SHA-256
  `cb75bb94dca396d14d856e001fa1ed3a7d8d6ac46cf8c5d60eed2ca902f033c0`.
  Its repositories are clean on `main` at
  `fa611a1e2c72f5469095c3460dc3268cd21765c8` and
  `a73c742f2a0e5934d6da12069599dfde2a17b00d`.
- Preparation and inspection started no model. Exact candidate/Codex
  identities, the required 32-minute source-writer authority, ten pending
  doctor-ready tasks, intact sentinels, zero operation/collector manifests,
  empty aggregate, and absence of an RC.6 launch record all pass without
  changing the suite. RC.1 through RC.5 remain immutable rejected history.
- This prepared root is not yet live authority. A fresh independent pass must
  review the exact four-file tracked scope and the retained prepared suite
  before controller commit/push. Only after that publication may a distinct
  explicit confirmation authorize the retained command in `.agent/HANDOFF.md`.
  No live launcher is added, and no tag, release, external-use approval, queue
  authority, or `EXT-20` completion follows from preparation.

## EXT-20 RC.6 Attestation Review And Suite-Preparation Gate (2026-07-25)

- Independent controller readback accepted exact attestation ref
  `226276f151ae389d06c0118a931596712fbc7cc1`, dedicated run/job
  `30155142491` / `89671812731`, artifact `8618790256` with digest
  `sha256:1e8d9b6161efb8ff04000eaba24e202eeddff625e443fc15728cf98cbaba95fa`,
  and companion ten-job CI run `30155142490`. The run, job, artifact, workflow,
  branch, head, timestamps, names, size, expiration, and conclusions agree
  with the durable record.
- Both RC.6 sealed bundles, both retained local attestation outputs, exact
  candidate authority, unchanged task ledger, and all three RC.5 immutable
  evidence streams reverified. The archive endpoint remains unauthenticated
  HTTP 401 with no configured read-only token, so no controller-side archive
  byte claim is added.
- Raw Git published the accepted three-file remote-attestation record on
  `main` as exact commit `c2c8c41e51bb4ebb6c65c93778b88ebad4a4eaa7`, tree
  `c81f4496984b02cec6fc41f7bea58c51f1b947f5`. This commit changes no release
  source, candidate/attestation ref, artifact, suite, or task authority.
- `agent-ext20-rc6-suite.sh` is the sole next no-model gate. It may change only
  the guarded suite's three RC candidate constants, perform the full static and
  Go verification, and prepare one collision-free persistent suite beneath
  ignored `.revolvr/` state. It grants no live/model call, live launcher, tag,
  release, external-use approval, queue authority, or `EXT-20` completion.

## EXT-20 RC.6 Remote Artifact-Attestation Authority (2026-07-25)

- Empty-expected raw Git created only
  `refs/heads/level1-v0.1.0-rc.6-attestation` at reviewed workflow commit
  `226276f151ae389d06c0118a931596712fbc7cc1`; exact remote readback agrees.
  Candidate source/ref remained `73f1f81f1c51d927114f19818a18161d0fcb8541`.
- Dedicated push run `30155142491` and sole job `89671812731` completed
  successfully at the exact workflow head. Its sole unexpired artifact is
  `8618790256`, `level1-v0.1.0-rc.6-attestation`, `70273424` bytes, digest
  `sha256:1e8d9b6161efb8ff04000eaba24e202eeddff625e443fc15728cf98cbaba95fa`.
  No read-only archive token was configured; the unauthenticated download
  endpoint returned HTTP 401, so remote artifact bytes are not claimed by the
  controller.
- Companion CI run `30155142490` completed successfully at the exact head with
  exactly its ten mandatory jobs: `89671812720`, `89671812726`, `89671812717`,
  `89671812747`, `89671812733`, `89671812719`, `89671812737`, `89671812730`,
  `89671812748`, and `89671812744`. Exact names, timestamps, URLs, and
  conclusions are retained in `.agent/STATE.md` and `.agent/HANDOFF.md`.
- Complete RC.6 and historical sealed-bundle, remote-ref/tag, retained local
  attestation, and RC.5 evidence checks passed before and after publication.
  RC.1 through RC.5 remain immutable rejected history. `.agent/TASKS.md`
  remains unchanged with `EXT-20` unchecked.
- This authority stops at remote ref/run/job/artifact and companion-CI
  evidence. Only a later separately authorized no-model pass may prepare a new
  collision-free RC.6 suite. No model call, live suite, tag, release,
  external-use approval, queue authority, or `EXT-20` completion is granted.

## EXT-20 RC.6 Attestation Workflow Review And Remote Gate (2026-07-25)

- Independent controller review accepted the four-file RC.6 attestation scope.
  It repeated workflow structure, exact constants, action pins, shell syntax,
  candidate/ref/artifact collision checks, complete sealed-bundle verification,
  exact candidate-CI and task-ledger readback, and the full unmodified workflow
  shell from a fresh detached remote clone. The independent replay root
  `/tmp/revolvr-ext20-rc6-attestation-review.OkDN5p` has exactly 29 output
  files, path-bearing content-stream SHA-256
  `6d6ae497c29da7a9722ccefa643f4240e7be7c8b6c9bcd6f8270da35c9e6f050`,
  six exact artifact hashes, and three byte-identical build pairs.
- Raw Git published only the reviewed scope on `main` as exact commit
  `226276f151ae389d06c0118a931596712fbc7cc1`, tree
  `eac5c8d4696cec6f5383d9fd19f3d482045028c8`, parent
  `f282f263d817ff4ab32e04fe86e3c42612d18ca9`. This commit is workflow
  authority; release source remains candidate
  `73f1f81f1c51d927114f19818a18161d0fcb8541`.
- `agent-ext20-rc6-attestation-remote.sh` is the sole next publication gate.
  Running it authorizes only empty-expected creation of
  `refs/heads/level1-v0.1.0-rc.6-attestation` at the exact workflow commit and
  collection of its dedicated run/job/artifact plus companion-CI evidence. It
  grants no `main` commit/push, suite, model, tag, release, external-use,
  queue, or `EXT-20` completion authority.

## EXT-20 RC.6 Local Artifact-Attestation Workflow Authority (2026-07-25)

- The only locally admitted RC.6 attestation implementation is separate
  workflow `.github/workflows/level1-rc6-candidate-attestation.yml`, SHA-256
  `708f2f35d2c9a71f803fc136f33a5bd4bbd65624de50af84caa98cd3a3395fdf`,
  triggered solely by a push to `level1-v0.1.0-rc.6-attestation`. Trigger HEAD
  is workflow authority only; release source remains exact candidate commit
  `73f1f81f1c51d927114f19818a18161d0fcb8541`, tree
  `7c9753461a08b25915f4f53533d91e57d40a20ca`, with source-CI run
  `30153462797` recorded in the retained authority manifest.
- Exact Go 1.26.5, two independent clean non-local clones with distinct build
  and module caches, and the settled release environment/flags must reproduce
  byte-identical Linux/Darwin/FreeBSD amd64 pairs and sealed hashes
  `f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d`,
  `596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7`,
  and `60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344`.
  Every retained artifact requires exact Go/tool/path/compiler/trimpath/
  target/CGO/source/clean-VCS metadata, an empty build ID, and exact
  `main.version=0.1.0` authority.
- The single future artifact authority name is
  `level1-v0.1.0-rc.6-attestation`. Before upload, the workflow requires exact
  workflow/ref identity, exact candidate-ref identity, no RC.6 tag or foreign
  attestation-namespace ref, and no existing artifact with that name. The one
  upload retains both binary sets plus all hash, build-metadata, build-ID,
  version, reproducibility, and exact manifest authority.
- Complete unmodified embedded-shell execution from a detached exact-source
  clone passed. Retained local root
  `/tmp/revolvr-ext20-rc6-attestation.9dtfzU` has exactly 29 attestation files
  and path-bearing content-stream SHA-256
  `fde643d9b1a262c087a661b1ef617d143414ce149e44a967221af7e21d93d7c1`.
  Both RC.6 sealed bundles, exact candidate/CI authority, historical refs and
  workflow hashes, and retained RC.5 evidence reverified before and after.
- Local construction grants no commit, push, attestation ref, remote run or
  artifact, suite, model call, tag, release, external-use approval, queue
  authority, or `EXT-20` completion. A later fresh independent review must
  accept the complete four-file scope before any separately authorized
  controller commit and empty-expected raw-Git publication of still-absent ref
  `refs/heads/level1-v0.1.0-rc.6-attestation`. RC.1 through RC.5 remain
  immutable rejected history.

## EXT-20 RC.6 Remote-CI Review And Attestation Handoff (2026-07-25)

- Independent raw-Git and public-REST readback accepted exact candidate ref
  `refs/heads/level1-v0.1.0-rc.6` at
  `73f1f81f1c51d927114f19818a18161d0fcb8541` and CI run `30153462797` as
  push-triggered, exact-head, completed, and successful with exactly the ten
  required unique successful jobs. Both sealed RC.6 bundles and all retained
  historical authority remained unchanged.
- Raw Git published the reviewed three-file remote-CI record on `main` as
  exact commit `716c9a2293555f9abee80d384a673be426fe2ce0`, tree
  `26c3039581cf12278ae5d58b58c0bd72fcd3e592`; exact local and remote readback
  agree. This commit is not candidate source.
- `agent-ext20-rc6-attestation.sh` is the next bounded local-only gate. It may
  add and fully exercise only the collision-free exact-candidate Go 1.26.5
  artifact-attestation workflow. It grants no workflow commit/push,
  attestation ref, remote run or artifact, suite, model call, tag, release,
  external-use, queue, or `EXT-20` completion authority.

## EXT-20 RC.6 Exact-Candidate Remote CI Authority (2026-07-25)

- The RC.6 remote candidate authority is exact branch
  `refs/heads/level1-v0.1.0-rc.6` at source
  `73f1f81f1c51d927114f19818a18161d0fcb8541`, tree
  `7c9753461a08b25915f4f53533d91e57d40a20ca`. It was created only after a
  no-tags fetch, exact local/remote `main` agreement at
  `4aa16b63f05da16d2d1620be83aec18a28e6d0cb`, ancestry of published
  local-candidate record `60a19d4e38ddd5ea76490221973601e0efce2625`,
  complete verification of both RC.6 sealed bundles, published-source
  reachability, and absence of every RC.6 candidate, attestation, tag,
  workflow, and remote artifact-name collision. Raw Git used the required
  empty-expected lease and exact remote readback matched; no local branch was
  created and no existing ref was moved or deleted.
- Push-triggered CI run `30153462797`, run number `66`, attempt `1`, event
  `push`, branch `level1-v0.1.0-rc.6`, and exact head SHA
  `73f1f81f1c51d927114f19818a18161d0fcb8541` completed with conclusion
  `success`. Its URL is
  `https://github.com/ponchione/revolvr/actions/runs/30153462797`.
  The jobs endpoint returned exactly the ten mandatory unique jobs, each at
  that head SHA with status `completed` and conclusion `success`:
  `89667615436` Go 1.22 source floor and tests,
  `89667615420` Production autonomous strict-fake suite,
  `89667615421` Race tests, `89667615427` Vet and module verification,
  `89667615433` Fake-Codex success smoke,
  `89667615415` Fake-Codex verification-failure smoke,
  `89667615483` Build linux/amd64, `89667615546` Build darwin/amd64,
  `89667615476` Build freebsd/amd64, and `89667615399` Build Windows
  diagnostic stub. Exact job URLs are retained in `.agent/STATE.md` and
  `.agent/HANDOFF.md`.
- Post-run preservation keeps the RC.6 candidate inventory/seal exact at
  `30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1` /
  `d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88`
  and verification inventory/seal exact at
  `9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf` /
  `f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f`.
  All ten historical RC.1-through-RC.5 remote refs still match their baseline,
  historical RC tags remain absent, and the RC.5 suite/evidence/launch-record
  content-stream hashes remain
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`.
  The RC.5 launcher remains permanently failed closed.
- Source CI is not artifact attestation. The next separate gate may only
  construct and locally verify a collision-free RC.6 workflow that checks out
  the immutable candidate SHA, uses exact Go 1.26.5, performs two clean
  Linux/Darwin/FreeBSD release builds, and compares their hashes and embedded
  identities with the sealed candidate. It grants no commit, push,
  attestation ref, remote run or artifact, suite, model call, tag, release,
  external-use approval, queue authority, or `EXT-20` completion.

## EXT-20 RC.6 Independent Local Review And Remote-CI Handoff (2026-07-25)

- Fresh independent review accepted the complete RC.6 candidate and
  verification bundles after exact seal/file-set verification, artifact hash,
  metadata, version, and empty-build-ID checks, source/tree/ancestry and
  remote-collision checks, vulnerability-evidence inspection, a fresh full Go
  suite on the retained exact candidate source, and historical preservation
  checks. The review changed neither bundle nor retained evidence.
- Raw Git published the three-file local-candidate record on `main` as exact
  commit `60a19d4e38ddd5ea76490221973601e0efce2625`, tree
  `d4d8bedccb5309f108f8d10a18c42c8fa9a750e6`; exact local and remote readback
  agree. This commit is not candidate source.
- `agent-ext20-rc6-remote.sh` is the next bounded gate. Running it authorizes
  only collision-safe creation of absent candidate ref
  `refs/heads/level1-v0.1.0-rc.6` at exact source
  `73f1f81f1c51d927114f19818a18161d0fcb8541` and read-only collection of its
  exact ten-job push-triggered CI result. It grants no attestation workflow,
  suite, model call, tag, release, external-use, queue, or `EXT-20` completion
  authority.

## EXT-20 RC.6 Local Candidate Authority (2026-07-25)

- The sole local RC.6 candidate authority is exact published source commit
  `73f1f81f1c51d927114f19818a18161d0fcb8541`, tree
  `7c9753461a08b25915f4f53533d91e57d40a20ca`, release version `0.1.0`,
  source floor Go `1.22.12`, and release Go `1.26.5`. Remediation commit
  `010a8939ef6ad889a34590d05ce0326b6df57571` is its ancestor, and no
  `cmd`, `internal`, `go.mod`, or `go.sum` delta follows that remediation.
  Later controller commit `7598be79d8cfcbaa7ead3e184c7cd1c71be29244` and
  `agent-ext20-rc6.sh` are not candidate source and are absent from candidate
  clones and artifacts.
- The immutable candidate bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51`.
  Linux, Darwin, and FreeBSD amd64 artifact SHA-256 values are respectively
  `f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d`,
  `596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7`,
  and `60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344`.
  Build-instructions SHA-256 is
  `94d291ec80db7427bddc1db57cac147c5d061ca3dbdbdd038259e1da3505a906`;
  the 15-file candidate inventory/seal values are
  `30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1` /
  `d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88`.
  Two independent fresh non-local clones produced byte-identical artifacts
  with module-readonly mode, disabled CGO, amd64 targets, trimpath, explicit
  clean VCS metadata, and empty Go build IDs.
- The separate immutable verification bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51-verification`.
  Its 50-file inventory/seal values are
  `9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf` /
  `f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f`.
  It retains exact source, tool, build, target, metadata, command, regression,
  full-suite, module, vulnerability, collision, historical-inventory, remote-
  ref, and RC.5 preservation evidence. Both bundles must be accepted only by
  complete manifest and file-set verification.
- Local verification found zero reachable and zero imported-package
  vulnerabilities. The sole module-only finding remains Windows-only, uncalled
  `GO-2026-5024` in `golang.org/x/sys@v0.30.0`, fixed in `v0.44.0`. Focused
  planner-role dossier and fallback-receipt regressions, Structured Outputs,
  lifecycle authority, production happy path, strict fake Codex, complete Go
  suites, vet, and module verification all passed; this is local evidence only
  and makes no real API-acceptance claim.
- RC.1 through RC.5 remain immutable rejected history. The RC.5 suite,
  evidence, and launch-record content-stream SHA-256 values remain
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`;
  the RC.5 live launcher remains permanently failed closed. RC.6 construction
  creates no authority to retry or reuse historical evidence.
- The next gate is fresh independent read-only review of the retained RC.6
  helper and both bundles. Candidate-ref publication and remote CI remain a
  later separately authorized task. No attestation workflow, suite, live-model
  operation, tag, release, external-use approval, queue authority, or
  `EXT-20` completion is granted. The attended developer-alpha documentation
  remains explicitly separate from release qualification.

## Developer-Alpha Publication And Level-1 Qualification Resumption (2026-07-25)

- Independent review accepted the six-file attended developer-alpha scope
  after repeating the disposable external smoke, focused production happy
  path, full Go suite, shell/diff checks, exact source-scope comparison, and
  immutable RC.5 evidence checks. Raw Git published it on `main` as exact
  commit `73f1f81f1c51d927114f19818a18161d0fcb8541`, tree
  `7c9753461a08b25915f4f53533d91e57d40a20ca`; local and remote readback agree.
- The operator then explicitly directed continued progress. The next bounded
  pass resumes ordered `EXT-20` qualification by constructing and locally
  verifying fresh collision-free candidate `level1-v0.1.0-rc.6` from that
  exact published source. It may not make a live model call, publish a ref,
  add an attestation workflow, tag, release, approve external use, or complete
  `EXT-20`.
- Developer-alpha use remains a separate attended, disposable/recoverable
  path. RC.1 through RC.5 remain immutable rejected history and cannot supply
  RC.6 artifacts, runtime state, or evidence.

## Attended Developer-Alpha Entry Path (2026-07-25)

- The supported developer-alpha entry is the documentation-only path in
  `docs/attended-developer-alpha.md`: from a clean current `main` with no
  product-source delta after remediation commit
  `010a8939ef6ad889a34590d05ce0326b6df57571`, build exactly
  `go build -trimpath -o ./bin/revolvr ./cmd/revolvr` and use the resulting
  `<source-root>/bin/revolvr` in one clean disposable or separately recoverable
  repository.
- Developer-alpha authority is limited to one foreground attended task with a
  unique operation ID, explicit `--max-cycles 3`, passing general and exact-
  task doctor checks, preserved evidence, and operator review of the generated
  commit before any separate integration. Queue, daemon, concurrent repository
  mutation, runtime-state edits, and automatic integration remain excluded.
- No new helper or product behavior is needed. The existing external attended
  shell smoke proves the CLI/init/config/status and fail-closed doctor surfaces
  in a collision-free disposable Git repository without executing its fake;
  `TestProductionAutonomousHappyPath` separately proves the complete ordinary
  production task composition with the reusable strict fake Codex. This split
  proof does not establish real API acceptance or Level-1 qualification.
- The next gate is independent read-only review of the documentation and
  durable records before any separately authorized publication. `EXT-20`
  remains unchecked; RC.1 through RC.5 remain immutable rejected evidence, and
  no RC.6, candidate, tag, release, live-model run, or external-use approval is
  implied.

## Attended Developer-Alpha Versus Release Qualification (2026-07-24)

- Publishing the known RC.5 source fixes is sufficient to explore a bounded
  attended developer-alpha path, but it is not evidence that EXT-20 or Level-1
  external-project qualification passed. Developer use must remain attended,
  finitely bounded, and limited to disposable or recoverable repositories with
  generated commits reviewed before integration.
- The next pass will simplify the existing build/init/check/run/status path and
  verify it with strict fake Codex. It will not create RC.6 or change product
  behavior. Full real-Codex qualification remains an optional later track that
  requires explicit operator direction.
- RC.1 through RC.5 and their evidence remain immutable rejected history. The
  developer-alpha path cannot reuse their suites or live launchers and conveys
  no tag, release, or external-use approval.

## EXT-20 RC.5 Terminal Source-Remediation Authority (2026-07-24)

- Planning application accepts exactly two dossier forms: the legacy
  `autonomous-task-dossier-manifest-v1` authority retained for compatibility,
  or `autonomous-role-dossier-manifest-v1` with an exact planner projection.
  Legacy supervisor identity remains an exact schema/hash/size comparison;
  role-projected cycles retain nonblank supervisor authority and exact worker
  dossier/task/hash/size/source/output provenance. Missing projections,
  malformed projection schemas, wrong roles, unknown schemas, and any identity
  mismatch fail closed. No dogfood-specific identity is trusted by product
  code.
- Receipt task text is an exact identity field, not a normalized identifier.
  Fallback construction and ordinary parsing preserve every byte of a nonblank
  task body, including its final newline; whitespace-only input still receives
  the existing fallback, while run/pass/task identifiers, verdict, status, and
  commit fields keep their intentional normalization. The harness's fixed
  no-verification sentence is human text rather than a command claim, while
  structured verification entries remain authoritative.
- `agent-ext20-rc5-live-direct.sh` is permanently retired terminal authority.
  Every invocation returns the same refusal before inspecting or mutating any
  suite or launch record. The retained launch status 130 and later child
  `unsafe_or_ambiguous` outcome remain distinct immutable evidence. Any future
  live candidate must receive a separately designed and reviewed foreground/
  visible child boundary; this remediation provides no live or candidate
  authority.
- Fresh independent review accepted the remediation after ensuring the fixed
  no-verification sentence is plain human section text, not a parser claim or
  literal parser exception. The operator authorized raw-Git publication in
  this session. It grants no candidate construction, tag, release, external-
  use approval, or `EXT-20` completion.

## EXT-20 RC.5 Terminal Live-Evidence Policy (2026-07-24)

- A retained launch record plus a validated first-operation manifest proves
  that replacement suite `ext20-9b7cc650ea0c` entered its live path once. The
  launcher's retained interruption status 130 and the descendant operation's
  later terminal `unsafe_or_ambiguous` result are distinct facts; neither may
  overwrite or be inferred from the other.
- Any live start makes a prepared suite single-use. Its later prepared-content
  mismatch is therefore an expected reuse refusal, not missing evidence and
  not authority to rebuild or retry in place. RC.5, its suite, operation,
  launch record, and evidence are immutable rejected history.
- The failure establishes three product regressions that must be repaired
  before any later candidate: planning application must validate the exact
  planner role-projected dossier authority; fallback receipts must preserve
  exact nonblank task bytes; and a no-verification placeholder must not parse
  as a verification claim. Fixes must be general, fail closed, and covered by
  focused tests; no dogfood-specific bypass is permitted.
- The RC.5 direct launcher must be permanently retired. A later candidate's
  live boundary requires separate construction and review and must keep child
  progress visible while retaining unambiguous signal and terminal status.
  Source remediation grants no candidate, live, release, or external-use
  authority.

## EXT-20 RC.5 Diagnostic-Retaining No-Start Replacement (2026-07-24)

- The first persistent RC.5 suite at
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite`
  is immutable retired authority after an unproven no-start. Its exact bytes,
  hashes, clean repository heads, pending tasks, sentinels, and empty evidence
  remain preservation evidence only; no command may execute its live path.
- Exactly one replacement-specific parent was created at
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc5-no-start-replacement.PNKJ20`.
  Its `suite` child is the sole newly prepared authority: suite ID
  `ext20-9b7cc650ea0c`, authority SHA-256
  `3af0ccb8a7dd7fa5c2205882ce63c03dcb59935cb297fd4804a6a544a4827289`,
  operation-plan SHA-256
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`,
  and path-bearing content-stream SHA-256
  `44a0bacaf8c9ba0c52e3724553652dedda677a4118947e34b44fefa38671cee7`.
  Preparation may retain exact Codex `0.144.4` package bytes but conveys no
  model or live authority.
- The direct live launcher binds only the new root, exact suite authority, and
  clean repo heads. Its read-only `--check` path creates no diagnostic. After
  every preflight passes, only the separately confirmed live path may reserve
  one atomic collision-free record beneath ignored
  `.revolvr/ext20-rc5-launch-records`. Before invoking the guarded suite once,
  that record must contain exact pre-start authority, stdout/stderr files, and
  an initial status. Normal exit, launcher failure, or trappable interruption
  replaces status with the retained terminal classification; a collision
  preserves the existing record and refuses execution.
- This no-model preparation required a fresh independent review followed by
  separately authorized publication and a clean published `--check`. It grants
  no tag, release, external-use approval, or `EXT-20` completion authority.
- The fresh `agent-ext20-rc5-no-start-review.sh` pass left the accepted scope
  unchanged; independent controller scope/root/no-token/full-Go checks also
  passed.
- Exact token `EXT20_PUBLISH_RC5_NO_START_REPLACEMENT` published only the
  reviewed seven-file scope to raw-Git `main` as commit
  `d4e00d99252888528db29785eefedbf9f087859a`, an exact child of
  `239008cfbe819e65c1b2f886a9ba81323e5643e9`. Exact local/remote readback and
  the clean no-model/no-launch-record preflight passed.
- The published `agent-ext20-rc5-live-direct.sh` is the sole next launcher and
  still requires separate exact live-model confirmation. Publication grants no
  live, tag, release, external-use, or `EXT-20` completion authority.
- Controller-record commit `8b87d0b87ad4cdf031b1b128871ad9d76bd4364e`
  is the exact three-state-file direct child of the replacement commit.
  Independent local/remote readback, clean-tree, focused no-token, exact-root,
  and no-model/no-launch-record preflight checks pass from that published
  state.

## EXT-20 RC.5 Unproven Live No-Start Policy (2026-07-24)

- After the operator reported completing the exact token-bearing command, the
  prepared suite remained byte-for-byte pristine with no running process,
  launch log, operation/collector manifest, aggregate, task transition,
  repository mutation, or retained exit status. This is an unproven no-start;
  it grants neither model/API acceptance nor evidence of a product rejection.
- Because explicit live authority was exercised but no outcome was retained,
  the exact suite is retired and must never be reused. The direct launcher is
  failed closed immediately. Candidate/attestation authority remains unchanged
  and may support only a new collision-free no-model prepared suite.
- `agent-ext20-rc5-no-start-remediation.sh` is the sole next launcher. It may
  prepare one persistent replacement suite and a diagnostic-retaining future
  live boundary, but grants no commit, push, model call, tag, release,
  external-use approval, or `EXT-20` completion authority.

## EXT-20 RC.5 Persistent Prepared-Suite Recovery (2026-07-24)

- The published direct launcher failed closed before any model call because
  external cleanup removed its `/tmp` prepared root. The older RC.1-through-
  RC.4 `/tmp` roots were also absent. Historical Git, sealed bundles, remote
  evidence, hashes, and durable summaries remain immutable, but the missing
  volatile filesystem copies are no longer described as retained.
- A single replacement RC.5 suite is prepared without model calls at exact
  ignored runtime path
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite`.
  Keeping the replacement under repository runtime state avoids treating
  volatile `/tmp` as release-gate evidence authority. Candidate, Codex, plan,
  repositories, tasks, sentinels, and empty runtime evidence remain otherwise
  unchanged in meaning.
- `agent-ext20-rc5-live-direct.sh` is rebound to the replacement's
  exact root, authority/content hashes, and repository heads. This recovery
  grants no live-model, tag, release, external-use, or `EXT-20` completion
  authority.
- The fresh `agent-ext20-rc5-recovery-review.sh` pass left the reviewed tree
  unchanged. Independent controller checks repeated exact scope, root,
  authority, shell/static, remote-ref, and full Go verification successfully.
- Exact `EXT20_PUBLISH_RC5_RECOVERY` authority admitted only publication of the
  reviewed six-file recovery and its controller record to raw-Git `main`, plus
  the clean published no-model preflight. The recovery is published as exact
  commit `d0bde8dffd8e233c04e593519546b7634d836304`, with parent
  `49dafe186f4c081c49483c46c6487914b1fd9c00`; remote readback matched and
  `./agent-ext20-rc5-live-direct.sh --check` passed without a model call. This
  grants no live operation, tag, release, external-use approval, or `EXT-20`
  completion authority.
- Controller-record commit `c896ebb81cf6168c21358b03fa6731ba43029663` is the
  exact direct child of the recovery commit and changes only the three durable
  state files. Independent local/remote readback and the no-model direct
  preflight pass from that clean published state. The already published direct
  launcher is therefore the sole next gate and still requires its distinct
  live confirmation token.

## EXT-20 RC.5 Prepared-Suite Review And Direct Live Gate (2026-07-19)

- Independent controller review accepted the exact three-line RC.5 suite
  authority change, both sealed bundles, static/collector checks, full Go
  suite, prepared checksum, candidate/Codex identities, clean repository
  heads, ten pristine pending task states, sentinels, and empty runtime
  evidence. The tracked preparation result is raw-Git published on `main` as
  `f5ba71d3be8c13b201c7609101a5269b6f463af5`.
- `agent-ext20-rc5-live-direct.sh` is the only admitted next launcher. Its
  `--check` mode repeats the complete fail-closed preflight without model
  calls. Actual live execution additionally requires the exact explicit token
  `EXT20_LIVE_REAL_CODEX_MODEL_CALLS`, uses only retained root
  `/tmp/revolvr-ext20-rc5.weLZtI/suite`, and executes the guarded suite once.
  The launcher is published at exact commit
  `d3872c00c30e15cc92dfbae8b890602c05b5fe8a`; its no-model `--check` passed
  after raw-Git publication.
- Any drift, failed start, or interruption fails closed and becomes terminal
  retained evidence; the suite or an operation must never be retried. The
  launcher grants no tag, release, or external-use approval. `EXT-20` can be
  completed only from a successful fully verified retained aggregate.

## EXT-20 RC.5 Prepared-Suite Authority (2026-07-19)

- The guarded external Level-1 suite now admits only RC.5 source
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, Linux artifact SHA-256
  `1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043`,
  and bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61`.
  Release output remains `revolvr 0.1.0`; exact Codex 0.144.4 authority and
  every plan, schema, scenario, threshold, configuration, and confirmation
  rule remain unchanged.
- The sole prepared RC.5 live-suite authority is
  `/tmp/revolvr-ext20-rc5.weLZtI/suite`, suite ID
  `ext20-f87a569b5efa`, authority SHA-256
  `6577bd6c433db64178f5406b62c554370b64f030523c28c0486c6f35fc779b7e`,
  and operation-plan SHA-256
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`.
  It has two clean disposable repositories, ten ready tasks across the exact
  11-row plan, effective 32-minute source-writer authority, zero operation or
  collector manifests, and an empty aggregate.
- Preparation and read-only inspection grant no live-model authority. A later
  pass requires separate confirmation and must use the exact retained root
  and command recorded in `.agent/HANDOFF.md`; any authority drift fails
  closed and requires a new collision-free suite. RC.1 through RC.4 remain
  immutable rejected history, and `EXT-20` remains unchecked. No tag,
  release, or external-use approval exists.

## EXT-20 RC.5 Attestation Review And Suite Launcher (2026-07-19)

- Independent controller review accepted the exact workflow specialization,
  retained local output, raw-Git commit/ref readbacks, dedicated remote
  run/job/artifact, and companion ten-job CI result. The remote attestation
  result is published on `main` at controller commit
  `6d32de0c9c932e202ea37ecb9d435fd70ad013ad`; this controller state commit is
  not candidate or workflow authority.
- `agent-ext20-rc5-suite.sh` is the sole next launcher. It authorizes only the
  minimal RC.4-to-RC.5 suite-authority update, no-model verification, and one
  fresh collision-free prepared RC.5 suite root. It may install the already
  fixed Codex package but may not start a model or commit/push.
- The later confirmation-gated live suite, tag, release, external-use
  approval, and `EXT-20` completion remain separate gates. Terminal RC.4 and
  all earlier evidence remain immutable.

## EXT-20 RC.5 Remote Artifact Attestation Authority (2026-07-19)

- The reviewed RC.5 workflow and local attestation state are published on
  `main` as exact commit `109b38cdb309b50c38ab2ef0df33998e92dfd5e6`.
  Raw Git created only the previously absent
  `refs/heads/level1-v0.1.0-rc.5-attestation` at that commit with an
  empty-expected lease. Candidate authority remains
  `refs/heads/level1-v0.1.0-rc.5` at exact source
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`.
- Push-triggered attestation run `29698647782` and sole job `88223716039`
  completed successfully at the workflow commit. Its sole artifact is ID
  `8445792045`, name `level1-v0.1.0-rc.5-attestation`, size 70,270,595 bytes,
  and digest
  `sha256:ab0febbc035f634d39babb897edd0c94bfaf1805ebc212e767a551fb1758b0e2`.
  The same push's exact ten-job CI run `29698647807` also completed
  successfully.
- Public REST metadata and the successful workflow job establish remote
  artifact retention and its in-workflow exact binary/hash/metadata checks.
  The unauthenticated archive endpoint returned HTTP 401 and no GitHub token
  was available, so no controller-side archive byte comparison is claimed.
- RC.5 candidate source CI and remote artifact-attestation prerequisites are
  complete. The next separately bounded gate is no-model preparation of a
  fresh guarded Level-1 suite bound to exact RC.5 source, Linux artifact, and
  bundle authority. This grants no live-model, tag, release, external-use, or
  `EXT-20` completion authority.

## EXT-20 RC.5 Local Artifact-Attestation Workflow Authority (2026-07-19)

- The only locally admitted RC.5 attestation implementation is the separate
  `.github/workflows/level1-rc5-candidate-attestation.yml`, triggered solely
  by a push to `level1-v0.1.0-rc.5-attestation`. Trigger HEAD is workflow
  authority only; release source remains exact candidate commit
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`.
- Exact Go 1.26.5, two independent clean non-local clones with distinct build
  and module caches, and the settled release environment/flags must reproduce
  byte-identical Linux/Darwin/FreeBSD amd64 pairs and sealed hashes
  `1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043`,
  `a0ba1e05f76d92c1d20577c897a37bc2b4a3252a4e0fb10ef9d736f25b07645d`,
  and `f9b6da20be9497c5eb772f7b40945fceedc064ecb6e081809c9510d71462e2d6`.
  Each retained binary carries independently checked tool/path/compiler/
  trimpath/target/CGO/source/clean-VCS metadata, an empty build ID, and exact
  `main.version=0.1.0` authority.
- The single remote artifact authority name is
  `level1-v0.1.0-rc.5-attestation`. Its authority manifest binds the workflow
  path, attestation ref and namespace, artifact name, candidate ref/ID/source/
  tree, toolchain, environment, flags, clean source passes, targets, and exact
  hashes. Both binary sets and all hash, metadata, version, build-ID, and
  reproducibility evidence remain in that one upload.
- Local construction and complete detached-source execution do not grant a
  commit, push, remote run/artifact, suite, live-model call, tag, release,
  external-use approval, or `EXT-20` completion. A later controller gate must
  independently verify and commit the workflow, publish only previously
  absent raw-Git ref `refs/heads/level1-v0.1.0-rc.5-attestation` with an
  empty-expected lease, preserve candidate ref
  `refs/heads/level1-v0.1.0-rc.5` at the exact candidate SHA, and collect exact
  run/job/artifact readback. RC.1 through RC.4 remain immutable rejected
  history.

## EXT-20 RC.5 Remote-CI Review And Attestation Launcher (2026-07-19)

- Independent controller readback accepted exact RC.5 candidate ref
  `refs/heads/level1-v0.1.0-rc.5` at
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`. Public REST independently
  confirmed run `29697069305`, run number `42`, attempt `1`, event `push`,
  exact branch/SHA, and the recorded ten unique jobs all at `completed` /
  `success`. Both sealed RC.5 bundle inventories passed again.
- The remote-CI result is raw-Git published on `main` at controller commit
  `1cd46ad7d0c240da378522c9540b421f39376f58`; this is durable state only and
  is not candidate source.
- `agent-ext20-rc5-attestation.sh` is the sole next launcher. It authorizes
  only local construction and verification of the collision-free RC.5
  exact-candidate Go 1.26.5 artifact-attestation workflow. It grants no commit,
  push, ref, remote run/artifact, suite, model call, tag, release,
  external-use approval, or `EXT-20` completion.

## EXT-20 RC.5 Exact-Candidate Remote CI Authority (2026-07-19)

- Remote candidate authority is exact branch
  `refs/heads/level1-v0.1.0-rc.5` at source
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`. The branch was created only
  after a no-tags fetch, exact published-source ancestry proof, empty local and
  remote candidate/attestation/tag namespaces, sealed-bundle verification,
  and exact historical-ref preservation. Its exact remote SHA readback, not
  the branch name alone, is authority.
- Push-triggered `ci.yml` run `29697069305` is the EXT-15 source-CI authority
  for RC.5. It binds event `push`, branch `level1-v0.1.0-rc.5`, exact source
  SHA, and exactly ten mandatory unique jobs to `completed` / `success`.
  Exact run and job IDs, URLs, head SHA, status, and conclusions are retained
  in `.agent/STATE.md` and `.agent/HANDOFF.md`.
- RC.1 through RC.4 remain immutable rejected history. Post-run verification
  preserved every sealed historical inventory and workflow, all eight
  historical remote refs, and the terminal RC.4 operation evidence; the
  40-target content/metadata fingerprint was unchanged.
- Source CI is not release-artifact attestation. The next separate gate may
  only construct and locally verify a collision-free RC.5 workflow that checks
  out the immutable candidate SHA, uses exact Go 1.26.5, reproduces two clean
  Linux/Darwin/FreeBSD release build passes, and verifies the sealed hashes and
  embedded identities. Candidate publication grants no attestation ref,
  remote artifact, suite, live-model, tag, release, external-use approval, or
  `EXT-20` completion authority.

## EXT-20 RC.5 Independent Local Review And Remote-CI Launcher (2026-07-19)

- Independent controller review accepted the sealed RC.5 candidate and
  verification bundles. Both inventories and seals, exact source/tree,
  artifact and build-instructions hashes, absolute-path binary metadata,
  empty build IDs, version symbols, historical preservation evidence, and the
  full Go suite plus focused race/app checks passed. The local candidate state
  is published on `main` at controller commit
  `13973d8952d5de3ad20c5e13a7e6a419c8d8b9e2`; this controller commit is not
  candidate source.
- `agent-ext20-rc5-remote.sh` is the sole next launcher. Running it grants one
  narrow external mutation: create the still-absent
  `refs/heads/level1-v0.1.0-rc.5` at exact source
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7` with an empty-expected raw-Git
  lease. The pass must then read back that exact ref and require its exact
  push-triggered ten-job CI run to succeed.
- The launcher grants no `main` publication, attestation workflow/ref, suite,
  model call, tag, release, external-use approval, or `EXT-20` completion.
  Those remain separate gates, and RC.1 through RC.4 remain immutable history.

## EXT-20 RC.5 Local Candidate Authority (2026-07-19)

- The sole local RC.5 candidate authority is exact published source commit
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`, release version `0.1.0`, and Go
  `1.26.5`. Later controller state/helper commits are not candidate source, and
  `agent-ext20-rc5.sh` must remain outside every candidate clone and artifact.
- The immutable local candidate bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61`; its inventory
  SHA-256 is
  `ba718e4bef733a370cff72570b96e3c2f0db0af4b9ad8eedc77db2c965ca0b88`.
  Its Linux, Darwin, and FreeBSD amd64 artifacts are respectively
  `1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043`,
  `a0ba1e05f76d92c1d20577c897a37bc2b4a3252a4e0fb10ef9d736f25b07645d`,
  and `f9b6da20be9497c5eb772f7b40945fceedc064ecb6e081809c9510d71462e2d6`.
  Two independent non-local clean clones produced byte-identical artifacts.
- The separate immutable verification bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61-verification`;
  its inventory SHA-256 is
  `e57353d8b929758b44d234458dfb2c3b4bae0cf347eccc206ba9424312a0e366`.
  It retains exact source/tool/build instructions, focused lifecycle and
  Structured Outputs guards, production happy-path and strict-fake proof,
  full source-floor/release checks, vulnerability results, independent binary
  metadata, collision checks, and historical preservation evidence.
- This local construction grants no candidate-ref publication, remote CI,
  attestation workflow, live suite/model operation, tag, release, external-use
  approval, or `EXT-20` completion. RC.1 through RC.4 and RC.4 terminal suite
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite` with operation
  `ext20-2bd21aea4f72-01` remain immutable rejected history. The next gate is a
  separate independent read-only review followed by explicitly authorized raw-
  Git publication of the collision-free RC.5 candidate ref and remote CI.

## EXT-20 Lifecycle-Authority Repair Publication (2026-07-19)

- Independent review accepted and raw Git published the lifecycle-authority
  repair as exact source commit
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`. This commit, not later
  controller state or launcher commits, is the sole permitted source for local
  RC.5 construction.
- Publication verification repeated formatting, focused ordinary/race tests
  for policy/supervisor/cycle, production happy-path and strict-fake ordinary/
  race tests, the complete Go suite, diff hygiene, and exact RC.4 terminal
  evidence/candidate preservation. Raw-Git pre-push parent and post-push
  readback matched.
- Publication grants only a later collision-free local RC.5 construction gate.
  It grants no candidate ref, remote CI, attestation, suite, live-model, tag,
  release, external-use, or `EXT-20` completion authority.

## EXT-20 Lifecycle Routing Authority (2026-07-19)

- `internal/autonomouspolicy` owns the versioned deterministic lifecycle
  routing projection `autonomous-lifecycle-routing-authority-v1`. `pending`
  admits exactly `plan`, `block`, and `needs_input`; `ready` admits the settled
  global action vocabulary in canonical order. Planning, working, verifying,
  auditing, correcting, needs-input, finalizing, completed, blocked,
  cancelled, superseded, and abandoned admit no new supervisor routing.
  Unknown lifecycles also fail closed.
- Runtime lifecycle enforcement and model-facing authority use that same
  projection. The autonomous cycle requires an open projection before calling
  the supervisor, passes the exact current lifecycle, and rejects a supervisor
  result that does not retain the same authority. The existing policy gate
  remains authoritative and does not admit `implement` directly from
  `pending`.
- The exact routing projection appears after the global supervisor profile in
  every prompt and is retained in supervisor provenance. Supervisor provenance
  advances to `revolvr-supervisor-provenance-v2`; the Structured Outputs action
  and profile schema, strict parsing, and Go action/profile validation remain
  unchanged. A malformed, reordered, widened, or closed projection cannot be
  rendered as decision authority.
- This repair adds no fallback, coercion, retry, dogfood special case, worker,
  verification, source, attempt, commit, or release authority. RC.4 and
  operation `ext20-2bd21aea4f72-01` remain immutable rejected history. A
  separate independent review and explicitly authorized raw-Git publication
  must pass before any collision-free RC.5 candidate may be constructed.

## EXT-20 RC.4 Terminal Lifecycle-Authority Failure (2026-07-19)

- RC.4 live suite `/tmp/revolvr-ext20-rc4.DGg1pW/suite` is terminally rejected
  after its first operation `ext20-2bd21aea4f72-01`. Scenario
  `successful-source-change-1` expected `completed` but retained
  `unsafe_or_ambiguous` because the supervisor chose `implement` while the
  task lifecycle was `pending`; the unchanged policy correctly admits only
  `plan`, `block`, or `needs_input` from pending.
- The product defect is an authority mismatch: the model-facing supervisor
  profile enumerates the global action vocabulary and the dossier reports
  lifecycle, but neither communicates exact lifecycle-admitted next actions.
  The model's decision was structurally valid and grounded, yet illegal under
  authority it was not shown. The repair must project lifecycle admission from
  one deterministic authority shared with policy enforcement, not weaken the
  gate, coerce decisions, retry, or add dogfood-specific behavior.
- The terminal evidence is valid and immutable. Exactly one supervisor pass
  completed, zero workers/attempts/verification/audits/commits ran, both
  control and workspace source stayed at
  `a75d4f059721ec7c9320650bd49d6d4cef9526cf`, and the outside sentinel stayed
  unchanged. RC.4 and its prepared suite must never be retried, reconciled,
  relabeled, or used as later candidate authority.
- `EXT-20` remains open. The bounded lifecycle-authority source repair is now
  locally complete; independent review and publication must precede any
  separate collision-free RC.5 candidate construction. No tag, release, or
  external-use approval exists.

## EXT-20 RC.4 First Confirmed Live No-Start (2026-07-19)

- Post-launch controller inspection found that
  `agent-ext20-rc4-live.sh` returned without entering the guarded suite. The
  exact retained root still has zero operation and collector manifests, an
  empty aggregate, unchanged clean repository heads, no new suite logs, and
  exact prepared content SHA-256
  `5e988363634a5aa4739c3b4bfccce865d2cf6e2c7ddb634aaa4eb25750641305`.
  No failed or interrupted suite/operation exists and no model acceptance or
  failure may be inferred from this no-start.
- The first wrapper retained no diagnostic, so its orchestration layer must not
  be rerun. A new `agent-ext20-rc4-live-direct.sh` performs deterministic
  shell preflight against the same immutable authority and executes the
  guarded suite directly only after a newly supplied exact confirmation
  argument. This is a new start against an untouched prepared root, not a retry
  of a failed operation.
- The direct launcher preserves the existing fail-closed rules: any authority
  drift or nonempty runtime evidence stops before live work; once the suite
  starts, failure or interruption is terminal and must never be retried.

## EXT-20 RC.4 Live-Suite Confirmation Gate (2026-07-19)

- Independent controller verification accepted the prepared RC.4 suite and
  published its exact three-constant authority update on `main` as
  `3284971acfc542fa64d600f7c40a58891b16cb7c`. Candidate bundle/static checks,
  the full Go suite, prepared checksum, exact candidate/Codex identities,
  repository heads/cleanliness, and zero-operation state all passed again.
- `agent-ext20-rc4-live.sh` is the sole next launcher. It fails before starting
  Codex unless invoked with the one exact argument
  `EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. Supplying that argument explicitly
  authorizes the guarded suite's real model calls against only
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite`; it grants no unrelated model work.
- The fresh live pass must reverify all recorded prepared authority before any
  call, execute the complete suite exactly once, preserve terminal evidence on
  either success or failure, and never retry a failed operation or suite.
  `EXT-20` may be checked only after the suite and independent retained-evidence
  verification satisfy every acceptance condition.
- This gate grants no controller commit/push, tag, release, or external-use
  approval. Those remain separately controlled after the live result.

## EXT-20 RC.4 Prepared-Suite Authority (2026-07-19)

- The guarded Level-1 suite now admits only RC.4 source
  `2546913e38ec273f64417dece2f91df78fd42fc2`, Linux artifact SHA-256
  `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
  and bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec`. The settled
  release output and exact Codex 0.144.4 path/version/hash authority, suite
  plan, scenarios, thresholds, configuration, and confirmation gate remain
  unchanged.
- The only prepared RC.4 live-suite authority is
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite`, suite ID
  `ext20-2bd21aea4f72`, with authority SHA-256
  `4f9b653c9e62e5fc5932b219952bbe61fccd79d331ac2bd7fcf2c570035eacb7`
  and operation-plan SHA-256
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`.
  It has two clean disposable repositories, ten ready tasks across the exact
  11-row plan, effective 32-minute source-writer authority, zero operation or
  collector manifests, and an empty aggregate.
- Preparation and read-only inspection grant no live-model authority. A later
  pass requires separate confirmation and must use the exact retained root and
  exact confirmation-gated command recorded in `.agent/HANDOFF.md`; any root,
  candidate, Codex, configuration, task, repository, or prepared-authority
  drift fails closed and requires a new collision-free suite.
- RC.1, RC.2, and RC.3 remain immutable rejected history. Their refs, bundles,
  workflows, hashes, artifact records, diagnostics, and any available roots
  cannot become RC.4 authority. `EXT-20` remains unchecked, and no tag,
  release, or external-use approval exists.

## EXT-20 RC.4 Remote Artifact Attestation Authority (2026-07-19)

- The reviewed workflow and local attestation state were committed on `main`
  as `52c2db07a86677e67921bcbfbcbdf26397b47615`. Raw Git then created only the
  previously absent `refs/heads/level1-v0.1.0-rc.4-attestation` at that exact
  workflow commit using an empty-expected force-with-lease. Candidate source
  authority remains `refs/heads/level1-v0.1.0-rc.4` at exact commit
  `2546913e38ec273f64417dece2f91df78fd42fc2`.
- Push-triggered attestation run `29690065853` and job `88201098277` completed
  successfully at the exact workflow commit. Its sole artifact is ID
  `8443312175`, name `level1-v0.1.0-rc.4-attestation`, size 70,214,949 bytes,
  and digest
  `sha256:0a3567ec0fbc31aff65424790402f81a20df3f22c49659854993dcbeb1eb8fbc`.
  The same push's ten-job CI run `29690065840` also completed successfully.
- Public REST metadata and the successful workflow job establish remote
  artifact retention and its in-workflow exact binary/hash/metadata checks.
  GitHub rejected unauthenticated archive download with HTTP 401, and no
  controller token was available, so no controller-side archive byte
  comparison is claimed.
- RC.4 has now satisfied its candidate-ref, exact-source CI, and remote
  artifact-attestation prerequisites. The next separately bounded gate is
  no-model preparation of a fresh guarded Level-1 suite using exact RC.4
  source, Linux artifact, and bundle authority. This grants no live-model,
  tag, release, external-use, or `EXT-20` completion authority.

## EXT-20 RC.4 Local Artifact-Attestation Workflow Authority (2026-07-19)

- The only locally admitted RC.4 attestation implementation is the separate
  `.github/workflows/level1-rc4-candidate-attestation.yml`, triggered solely
  by a push to `level1-v0.1.0-rc.4-attestation`. Trigger HEAD is workflow
  authority only; release source remains exact candidate commit
  `2546913e38ec273f64417dece2f91df78fd42fc2` and tree
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`.
- Exact Go 1.26.5, two independent clean non-local clones with distinct build
  and module caches, and the settled release environment/flags must reproduce
  byte-identical Linux/Darwin/FreeBSD amd64 pairs and the sealed RC.4 hashes.
  Each retained binary carries independently checked tool/path/compiler/
  trimpath/target/CGO/source/clean-VCS metadata, an empty build ID, and exact
  `main.version=0.1.0` authority.
- The single remote artifact authority name is
  `level1-v0.1.0-rc.4-attestation`. Its authority manifest binds the workflow
  path, attestation ref and namespace, artifact name, candidate ref/ID/source/
  tree, toolchain, environment, flags, clean source passes, targets, and exact
  hashes. Both binary sets and all hash, metadata, version, build-ID, and
  reproducibility evidence remain in that one upload.
- Local construction and full detached-source execution do not grant commit,
  push, remote-run, suite, live-model, tag, release, external-use, or
  `EXT-20` completion authority. A later controller gate must independently
  verify and commit the workflow, publish only previously absent raw-Git ref
  `refs/heads/level1-v0.1.0-rc.4-attestation` with an empty-expected lease,
  preserve the exact candidate ref, and collect exact run/job/artifact
  readback. RC.1, RC.2, and RC.3 remain immutable rejected history.

## EXT-20 RC.4 Attestation-Workflow Construction Gate (2026-07-19)

- Independent controller verification published the exact-candidate remote-CI
  state as `8c0379aa3fb6824fb56d4f3c1180f4cc411ada2a`. The candidate ref and source
  remain immutable at `2546913e38ec273f64417dece2f91df78fd42fc2`.
- Executing `agent-ext20-rc4-attestation.sh` authorizes only local construction
  and verification of a collision-free exact-source Go 1.26.5 attestation
  workflow. The controller retains all commit, push, attestation-ref, and
  remote-evidence authority for a later reviewed step.
- This gate excludes suite preparation, live/nested model work, tags, release,
  external-use approval, and completion of `EXT-20`.

## EXT-20 RC.4 Exact-Candidate Remote CI Authority (2026-07-19)

- Remote candidate authority is exact branch
  `refs/heads/level1-v0.1.0-rc.4` at source
  `2546913e38ec273f64417dece2f91df78fd42fc2`; the branch name without that
  readback SHA is not authority. It was created only after an immediate
  fetch-without-tags, source publication/ancestry proof, empty candidate lease,
  and RC.4 attestation-ref/tag collision checks.
- Push-triggered CI run `29688941202` is the EXT-15 source-CI authority for
  RC.4. It binds event `push`, branch `level1-v0.1.0-rc.4`, the exact source
  SHA, and exactly ten mandatory jobs to `completed` / `success` conclusions.
  The run URL is
  `https://github.com/ponchione/revolvr/actions/runs/29688941202`; exact job IDs
  and URLs are retained in `.agent/STATE.md` and `.agent/HANDOFF.md`.
- Source-floor cross-build success is not release-artifact attestation. The
  next separately bounded gate must publish a collision-free RC.4 workflow/ref
  that checks out the immutable candidate SHA, uses exact Go 1.26.5, performs
  two clean release build passes, verifies the sealed Linux, Darwin, and
  FreeBSD hashes plus embedded identities, and retains exact run/job/artifact
  authority.
- Candidate publication and CI success grant no live API acceptance, suite or
  real-model authority, tag, release, external-use approval, or completion of
  `EXT-20`. RC.1, RC.2, and RC.3 remain immutable rejected history.

## EXT-20 RC.4 Candidate-Ref And Remote-CI Gate Authority (2026-07-19)

- Independent controller verification satisfied the local-candidate review
  gate and published its durable state on `main` as
  `1917df5c374f8337a7bebb429478e7e16ea8420d`. Candidate source remains only
  `2546913e38ec273f64417dece2f91df78fd42fc2`; no later controller commit is
  candidate authority.
- Executing `agent-ext20-rc4-remote.sh` grants one narrow external mutation:
  create previously absent `refs/heads/level1-v0.1.0-rc.4` at that exact source
  with an empty-expected raw-Git force-with-lease. Collision, identity drift,
  or a nonempty ref fails closed. Existing refs may never be moved or deleted.
- The same bounded pass must identify an unambiguous push-triggered Actions run
  with the exact branch and source SHA and require all ten EXT-15 jobs to
  conclude success. Raw Git and read-only public REST evidence replace `gh`.
- This authority excludes `main` commit/push, attestation workflow/ref,
  suite preparation, live/nested model work, tags, release, external-use
  approval, and completion of `EXT-20`. Those remain later separate gates.

## EXT-20 RC.4 Local Candidate Authority (2026-07-19)

- Replacement candidate `level1-v0.1.0-rc.4` binds release version `0.1.0`
  only to published source commit
  `2546913e38ec273f64417dece2f91df78fd42fc2` and tree
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`. Later state/controller commits
  and `agent-ext20-rc4.sh` are not candidate source. Publication reachability
  from `origin/main` and helper exclusion are required evidence.
- RC.4 retains the settled EXT-18 construction authority: Go 1.22.12 is the
  tested source floor; release artifacts use exact Go 1.26.5, local toolchain
  selection, module-readonly mode, disabled CGO, amd64, trimpath, explicit
  clean VCS metadata, an empty Go build ID, and exact `main.version=0.1.0`.
  Linux, Darwin, and FreeBSD are the supported targets. Two independent
  non-local clean clones must produce byte-identical artifacts.
- The locally reproduced RC.4 SHA-256 values are Linux
  `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
  Darwin
  `042563f350b71ec8cd5be1b49fc9d948383caa28087c0a5689bd6eb12f3808ab`,
  and FreeBSD
  `128b9f8ced3038a51534da63b9d9ffbaa5ea7341e0ab8dd17102fba86084a8e6`.
  The pinned build-instruction SHA-256 is
  `5d87ff8eb5e89865729237dda500c8387ef5880b3c10ea0bd77f896938d606e9`.
- The ignored immutable candidate bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec/` has inventory
  SHA-256
  `3535d7a2b46a0dbd3101428b4177e4c46baabc29190e5b1c580d90e6ff033f5d`.
  Its separate verification bundle at the sibling `-verification` path has
  inventory SHA-256
  `75a2bcaba12d28d42a5012ad70995f4eb10363e250ec8028350e0802b0b8429c`
  and retains source publication, Structured Outputs guard, production happy
  path, Go-floor/full-suite, vet, module, vulnerability, metadata/version,
  build-ID, reproducibility, collision, raw-ref, and preservation proof.
- The focused Structured Outputs guard and production happy path are local
  regression evidence only and do not establish live API acceptance. RC.4
  local construction grants no candidate-ref, remote-CI, attestation, suite,
  live-model, tag, release, or external-use authority.
- RC.1, RC.2, and RC.3 remain immutable rejected history. None of their suites,
  evidence, operations, runs, refs, workflows, artifacts, hashes, bundles, or
  diagnostics may be retried, reconciled, relabeled, mutated, or used as RC.4
  candidate/live authority. RC.4's next gate is a separately authorized,
  collision-safe raw-Git publication of
  `refs/heads/level1-v0.1.0-rc.4` at the exact source SHA followed by the full
  EXT-15 remote CI matrix on that SHA.

## EXT-20 Structured Outputs Repair Publication Authority (2026-07-19)

- Explicit operator authority published the independently reviewed follow-up
  repair with raw Git as exact source commit
  `2546913e38ec273f64417dece2f91df78fd42fc2` and tree
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`. Pre-push fetch proved the remote
  had not moved; post-push fetch proved `HEAD` and `origin/main` matched.
- Publication verification repeated formatting, focused ordinary and race
  tests, the production autonomous happy path, the recursive four-builder
  Structured Outputs guard, the complete repository suite, staged-scope
  inspection, and diff hygiene. This satisfies the prior independent-review
  and explicit-commit gates for local RC.4 construction.
- RC.4 candidate source authority is exactly the published repair commit and
  tree above. Later controller-only state or launcher commits are not candidate
  source. Local candidate construction must use fresh clean clones of that
  exact commit and remain collision-safe.
- Publication grants no live/API acceptance, candidate-ref publication,
  remote-CI or attestation authority, live-suite authority, tag, release, or
  external-use approval. `EXT-20` remains open. RC.1, RC.2, and RC.3 remain
  immutable rejected history and none of their authority or evidence may be
  reused or mutated.

## EXT-20 Structured Outputs First-Repair Audit Failure And Follow-up (2026-07-18)

- RC.3 candidate source `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`
  is immutable rejected release history. Its first live operation
  `ext20-802d9db69596-01` and Codex run
  `019f761f-078d-7b81-932b-278339f2a000` reached no inference: the API
  rejected the supervisor response schema with HTTP 400
  `invalid_json_schema` because `properties.conflicts.uniqueItems` is not
  permitted. The `unsafe_or_ambiguous` outcome, zero attempts, zero
  verification, zero commits, and unchanged source/Git evidence are terminal
  facts, not a recoverable or relabelable operation.
- The retained suite `/tmp/revolvr-ext20-rc3.Qghf19/suite` and evidence bundle
  below `evidence/repo-a/01-successful-source-change-1` are permanently
  retired. They must remain byte-for-byte unchanged and must never be retried,
  reconciled, relabeled, or used as RC.4 input. RC.1 and RC.2 retain the same
  immutable-history authority.
- The first local repair removed unsupported composition and uniqueness
  keywords and passed focused, race, production strict-fake, and full tests,
  but it failed independent audit. It did not make every object strict-mode
  compatible: multiple objects omitted declared properties from `required`,
  correction and audit retained bare/unconstrained objects, and the regression
  guard was only a finite denylist. Test success from that first repair is not
  schema-compatibility or API-acceptance evidence.
- Current official OpenAI Structured Outputs and strict function-calling
  documentation is the compatibility authority for all four ordinary
  production builders: supervisor `DecisionOutputSchema`, planner
  `PlanningOutputSchema`, auditor `AuditOutputSchema`, and corrector
  `CorrectionOutputSchema`. Every model-facing object must declare concrete
  `properties`, set `additionalProperties` to exactly false, and require every
  declared property exactly once. Semantic optionals remain schema-required
  and use supported null or exact empty-array representations.
- The supported-subset regression guard is an explicit allowlist, not a
  denylist. It distinguishes schema keywords from property and `$defs` names,
  recursively checks every definition and branch, resolves local `$ref`
  targets, requires array `items`, enforces the mandatory object shape, and
  reports exact JSON paths. An unknown keyword fails closed.
- Schema-level conditional composition is not decision authority. Strict JSON
  decoding and Go validation retain action/profile pairing, conditional field
  presence, exact correction-authority exclusivity, and all domain decisions.
  Every semantic set formerly expressed with `uniqueItems` rejects duplicate
  model output without deduplicating or reordering it: finding and correction
  partition IDs, child dependency/tag/conflict IDs, needs-input option and
  independent-work identities, exact independent option identities, audit
  finding IDs, and verification-tier identities.
- Full tiered verification results are trusted host evidence and are not
  model-authored audit provenance. The audit prompt exposes a closed projection
  with `verification.summary.tiered` fixed to null; the apply boundary compares
  the projection and deterministically reattaches the exact trusted host result
  before canonicalization and persistence. The smaller final-gate projection
  remains closed, typed, validated, and compared.
- The repair grants no external-use or release authority. `EXT-20` remains
  open. RC.4 construction remains blocked until this follow-up dirty tree passes
  a new independent audit and is separately committed with explicit authority.
  No live-call/API-acceptance claim follows from local validation. RC.4
  construction, publication, attestation, and live use are outside this pass.

## EXT-20 RC.3 Replacement Candidate Authority (2026-07-18)

- RC.1 and RC.2 remain immutable rejected release history with all local and
  remote evidence preserved. RC.3 is a distinct collision-free candidate and
  binds release version `0.1.0` to exact source commit
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c` and tree
  `23c0d27fc62be5f41feb45192e74f1df8ecff3fa`, which include the verified
  source-writer-window repair. The controller helper `agent-ext20-rc3.sh` is
  not source authority and is excluded from the commit and clean clones.
- RC.3 retains the settled EXT-18 construction authority: Go 1.22.12 is the
  source floor; release artifacts use exact Go 1.26.5, local toolchain
  selection, module-readonly mode, disabled CGO, amd64, trimpath, explicit
  clean VCS metadata, an empty Go build ID, and exact `main.version=0.1.0`.
  Linux, Darwin, and FreeBSD are the supported targets. Two independent
  non-local clean clones must produce byte-identical artifacts.
- The locally reproduced RC.3 SHA-256 values are Linux
  `9e9c13f43977edf49e7e6385c595aa20a01c16308ddfea1c30455ea88252ae9b`,
  Darwin
  `ee6db29cfcbcbd2e645184927fc5d7348ed924d6036a46d7b23eae55b5b43fd4`,
  and FreeBSD
  `6a42dc423ab1975e8ea4296f56be6c9a3773d9ccfda1c57244a50261e64f368a`.
  The pinned build-instruction SHA-256 is
  `2deaa06d380dfd7d86277e2090229ba1d212e1bad85c6b3db6a83a31036c1405`.
- The ignored immutable candidate bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.3-a16ea1bdc1a4/` has inventory
  SHA-256
  `766856c2783073c8ffa10cbce0e3c0a9f8ebee4db1785c309f6c5680f5e5ddae`.
  Its separate verification bundle at the sibling `-verification` path has
  inventory SHA-256
  `006cb0b7f2215878e757ae8ee104bb1b88d5ae31b8661168bcf5c705d08353ff`
  and retains source-lock, Go-floor/full-suite, vet, module, vulnerability,
  metadata/version, reproducibility, collision, raw-ref, and preservation
  proof. Fail-closed partial assembly diagnostics are retained under the two
  recorded failure suffixes; they are not candidate or verification authority.
- Local construction grants no remote, dogfood, tag, or external-use
  authority. RC.3 cannot enter EXT-20 dogfood until raw Git publishes a new
  `level1-v0.1.0-rc.3` candidate ref at the exact source SHA, every EXT-15 job
  passes on that SHA, and a separate collision-free exact-checkout Go 1.26.5
  attestation reproduces the three recorded artifact hashes and embedded
  identities. None of those remote actions are part of local construction.

## EXT-20 Effective Source-Writer Window Authority (2026-07-18)

- `internal/lock.RequiredSourceWriterTimeout` is the single authority for the
  complete supervisor source lease: `CodexTimeout + 2*GitTimeout + 1m`. It
  validates positive inputs and fails before arithmetic overflow. Effective
  configuration, supervisor validation, and autonomous-cycle validation all
  use this calculation rather than retaining duplicate formulas.
- Effective run configuration derives an omitted source-writer timeout only
  after Codex and Git timeouts have reached their final values. A valid
  explicit timeout at or above the required window remains authoritative;
  negative or shorter explicit authority and an overflowing derived window
  fail closed. Default heartbeat authority is derived only after that final
  timeout, while valid explicit heartbeat authority remains unchanged.
- Config check and mode-aware doctor expose the final timeout/heartbeat and the
  required execution window. The default Level-1 30m Codex/30s Git authority
  is therefore 32m with a 10m40s heartbeat at every admission and execution
  boundary. Because this changes effective authority and fingerprints, the
  effective-run configuration schema advances from v7 to v8.

## EXT-20 RC.2 Replacement Candidate Authority (2026-07-17)

- RC.1 at `ed65049fba6bf82852fd406ebc17afa90a953e3f` and all of its local,
  remote, and failed-live evidence remain immutable rejected history. They
  cannot be relabeled, overwritten, or used as the replacement candidate.
- Replacement candidate `level1-v0.1.0-rc.2` binds release version `0.1.0` to
  exact clean source commit `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`.
  Its locally reproduced Linux, Darwin, and FreeBSD amd64 SHA-256 values are,
  respectively,
  `06c1258a947def8c53e03bfd79944bb002351358fc8dfecd35682ab7532b5010`,
  `05a15786dd1617d77ec671f420075922f6f9a78bf03de1245f03008f0960dee1`,
  and `5891c88e1e13f5a0a0e3452c15221981a187652c2e563a7b8b218b63c07d2a29`.
- RC.2 continues the settled EXT-18 procedure unchanged: Go 1.22.12 is the
  source floor; release artifacts use Go 1.26.5, local toolchain selection,
  module-readonly mode, disabled CGO, amd64, trimpath, explicit clean VCS
  metadata, empty Go build ID, and exact release version. Two independent
  non-local clean clones must produce byte-identical artifacts.
- The ignored local RC.2 candidate and verification bundles are immutable
  construction evidence, not remote or external-use authority. A first
  relative-output-path build was fail-closed by metadata verification and is
  retained unchanged under a failed diagnostic suffix. Only the subsequent
  absolute-path build is candidate authority.
- RC.2 cannot enter EXT-20 dogfood until a collision-free remote candidate ref
  binds the exact source SHA, every EXT-15 job passes on it, and a separate
  exact-checkout Go 1.26.5 attestation reproduces these three hashes and
  embedded identities. This local construction grants no push, tag, live
  model, or external-use approval.

## EXT-19 Remote Candidate And Attestation Authority (2026-07-17)

- The Level-1 candidate source authority is remote branch
  `level1-v0.1.0-rc.1` at exact commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f`; branch names alone are not
  authority. Push-triggered CI run `29612464054` binds all ten mandatory EXT-15
  job conclusions to that source SHA.
- Release-binary comparison is a separate exact-checkout attestation because
  the mandatory source-floor CI cross-builds do not use the release toolchain,
  version flags, or retained outputs. Workflow commit
  `a1afdd73a7bfb03e9e5ef361616604115f9db5b8` checks out the immutable candidate,
  reproduces the EXT-18 Go 1.26.5 builds, fails on any metadata or SHA-256
  difference, and retains the binaries, metadata, and hash manifest.
- Successful attestation run `29615752091` and its unexpired artifact digest
  `sha256:def158256b667447248a0370ee6e2dbe724b2dc1971216300e21751d706ff94f`
  complete remote build evidence for EXT-19. This evidence grants no tag or
  external-use approval; EXT-20 dogfood and EXT-21 release decision authority
  remain separate.

## EXT-18 Reproducible Level-1 Candidate Authority (2026-07-17)

- Candidate `level1-v0.1.0-rc.1` binds release version `0.1.0` to exact clean
  source commit `ed65049fba6bf82852fd406ebc17afa90a953e3f`. The binary carries
  the final proposed release version so subsequent dogfood and the possible
  `v0.1.0` decision evaluate the same bytes; the candidate label itself is not
  an approval tag.
- Release builds require exact official stable toolchain `go1.26.5`, local
  toolchain selection, module-readonly mode, disabled CGO, `amd64`,
  `-trimpath`, explicit VCS metadata, an empty Go build ID, and exact
  `main.version`. Supported Level-1 artifacts are Linux, Darwin, and FreeBSD.
  Two independent non-local clean clones of the exact commit must produce
  byte-identical artifacts for every target.
- The ignored `.revolvr/release-candidates/` bundle is local immutable dogfood
  input, not source authority. Its canonical manifest binds source, version,
  toolchain, targets, build-instruction hash, and artifact hashes; a sorted
  complete regular-file inventory plus separate inventory digest detects
  missing, extra, aliased, symlinked, or changed evidence. Embedded Go build
  metadata must independently show the exact toolchain, target, source commit,
  and `vcs.modified=false`.
- Go 1.22.12 remains the tested language floor while release artifacts use the
  patched Go 1.26.5 toolchain. A release scan blocks on any reachable or
  imported-package vulnerability. Module-only unreachable findings are
  retained separately; this candidate records `GO-2026-5024` in the
  Windows-only `golang.org/x/sys/windows` surface without treating it as a
  supported-platform or reachable finding.
- Candidate construction grants no external-use approval and performs no
  commit, push, or tag. EXT-19 still requires direct operator authorization to
  push the exact candidate commit and prove the mandatory remote CI jobs;
  EXT-20 and EXT-21 retain dogfood and immutable approval authority.

## EXT-17 Level-1 Dogfood Evidence Authority (2026-07-16)

- Real collection is opt-in and never initializes or authors the external
  fixture. Admission requires an exact clean non-bare Git top level, a tracked
  fixed-content disposable marker plus an exact canonical-path confirmation,
  a nonempty outside sentinel, and a new evidence directory outside both.
- Candidate authority is the exact binary SHA-256, exact version output, and
  clean Go build `vcs.revision`. Codex authority is the exact executable path,
  SHA-256, and version, repeated by candidate config inspection and admitted
  by exact-task attended doctor. The raw approved configuration SHA-256 and
  every documented Level-1 effective bound must also agree before operation or
  evidence publication begins.
- One invocation owns one exact task operation and emits schema
  `revolvr-external-level1-dogfood-manifest-v1`. Before/after Git, sentinel,
  canonical task, runtime, workspace, ledger/export, receipt, run, history,
  completion, resource, and typed-outcome evidence is retained under relative
  bundle paths. A sorted SHA-256 file inventory covers the manifest and every
  regular evidence file; a separate digest covers that inventory, and bundle
  verification rejects missing, extra, aliased, or changed evidence.
- Ledger export/verify/replay and receipt validation are ordinary explicit
  evidence operations after the task run. The collector never edits task-run,
  state, history, receipt, or recovery files to manufacture an outcome. A
  mismatch, unclassified result, sentinel change, identity drift, invalid
  receipt, or failed ledger validation retains the bundle and fails the
  collector.
- Fixture-only output is deterministic mechanism evidence, not real-Codex soak
  or release approval. It uses a separately built clean VCS-stamped candidate,
  fixed fixture authority, and no model execution. Its four input faults prove
  pre-admission refusal against exact source/Git/index and outside-sentinel
  snapshots without creating an evidence directory.

## EXT-15 Exact-Candidate Release CI Authority (2026-07-16)

- Release CI runs on every pushed branch and tag and on every pull request,
  without path filters. Its required jobs are independent and have no job-level
  conditions or dependency chains, so a release ref cannot turn a failed
  prerequisite into skipped downstream checks.
- The Go 1.22 source-floor job pins `1.22.x` with `GOTOOLCHAIN=local`. Separate
  jobs own the production autonomous strict-fake suite, full race suite,
  vet/module verification, successful and verification-failure fake-Codex
  smokes, supported Linux/Darwin/FreeBSD builds, and the unsupported Windows
  diagnostic-stub assertion. The existing supported/unsupported platform split
  remains explicit.
- Every job verifies checked-out `HEAD` equals `GITHUB_SHA` and publishes that
  exact identity in its job summary. Workflow success is therefore evidence
  about one source commit rather than a moving branch name; remote execution
  and required-check conclusions for the candidate remain the separate EXT-19
  gate.
- Mixed-pass smoke fakes emit an exact syntactically valid Codex CLI version so
  config inspection can retain diagnostic executable identity. Their unlisted
  bytes/version never become release-authorized autonomous authority, and the
  production strict-fake suite continues to use an exact test manifest through
  its existing unexported test seam.

## EXT-14 Production Interruption Recovery Ownership (2026-07-16)

- The durable task-run operation is the Level-1 quarantine boundary. A machine
  interruption at any production task transition leaves the stable outer
  operation in flight; restart of that exact operation terminalizes it as
  `unsafe_or_ambiguous` without inferring whether an inner process or durable
  effect may be repeated.
- Notification delivery remains an independent at-least-once owner. Restart
  reuses the payload's stable delivery ID; a completed delivery journal is
  terminal and cannot invoke the receiver again.
- Explicit archive administration rolls its exact operation forward. An
  admitted journal whose manifest is not yet published may reconstruct the
  manifest only from still-active terminal authority and must match its
  journal-bound artifact identity. If immutable publication already happened,
  those published bytes are authoritative even when the journal still records
  the earlier stage.
- Failure-injection hooks are nil by default and bracket the production owner
  calls. They add deterministic interruption proof without changing ordinary
  execution or creating a generic retry surface.

## EXT-13 Explicit Operator Recovery Authority (2026-07-16)

- Autonomous task recovery exposes one read-only projection with seven ordered
  authorities: task, state, workspace, Git, ledger, receipt, and artifacts.
  The projection is the sole basis for reconciliation readiness and carries a
  canonical digest of the inspected operation and authority results.
- Workspace/Git recovery inspection is distinct from mutation-capable reopen.
  It verifies the same deterministic marker, common-directory, registration,
  linked-worktree, branch, HEAD, tree, source, and cleanliness authority but
  takes no Git-administration mutation lease and never publishes a retained
  ambiguity ref. Reopen retains that publication behavior for its explicit
  recovery owner.
- Reconciliation is restricted to terminal `unsafe_or_ambiguous` operations
  and requires both an explicit reconcile flag and an exact operation-ID
  confirmation. Generic retry and unblock remain separate surfaces and cannot
  invoke or clear this quarantine.
- All authorities are repeated under the execution lease. Reconciliation
  refuses any change, preserves the old operation byte-for-byte, and publishes
  a distinct admitted operation with a stable identity and evidence linking the
  old operation and authority digest. Exact repeated requests replay that new
  identity without duplicating authority.
- The daemon's fully-unattended safety mode is a request-level prerequisite and
  is therefore checked before environment-dependent executable admission.
  Requests that satisfy it still undergo the complete external configuration,
  executable identity, repository, and runtime admission sequence.

## EXT-11 External Git Containment Edge Matrix (2026-07-16)

- External Git containment is proved against real repositories at the app
  boundary. Dirty and staged operator authority and active submodules stop at
  shared admission; policy-relevant ignored source may pass porcelain
  cleanliness but stops at the content-sensitive source snapshot before task
  ref or workspace publication.
- SHA-1 and SHA-256 repositories use the same exact task-workspace and
  path-scoped commit contract. A control root may itself be a genuine linked
  worktree; its branch, HEAD, index, and source remain separate from the
  deterministic task branch and execution worktree.
- A concurrent operator commit on the ambient control branch is independent
  authority. Injection after the task commit's pre-HEAD observation and before
  task staging proves the task commit retains the admitted baseline parent and
  exact run-owned tree, while the operator commit retains its own ref/tree and
  neither absorbs the other's path.
- Index authority is its exact bytes, size, file type, permissions, and link
  count plus the resulting staged tree/status. The index inode timestamp is not
  authority because ordinary read-only Git status may refresh it without
  changing index bytes. Outside and unrelated-worktree sentinels retain the
  stricter complete metadata oracle, including timestamps, symlink targets,
  and hard-link counts.

## EXT-10 Run-Owned Commit And Git-Operation Containment (2026-07-16)

- Generated commits use one exact path authority twice: byte-sorted,
  duplicate-free paths from the complete admitted post-run capture are staged
  with literal pathspecs and supplied again to literal `git commit --only`.
  Unrelated paths already present in the index therefore remain staged but do
  not enter the generated commit; unrelated tracked-worktree and untracked
  bytes likewise remain outside it.
- Exact containment proof is based on real Git, not only command arguments.
  It compares the complete committed path tree and every changed/unowned blob
  after injecting late staged, tracked, and untracked operator bytes, and then
  proves those late filesystem and index authorities remain intact.
- External production autonomy does not own repository integration or
  destructive restoration. Push, merge, rebase, reset, clean, and stash are
  prohibited; the unused workspace `Restore` implementation and its private
  reset/clean support path are removed. Existing restored-status decoding
  remains compatible evidence, but no production operation may create new
  restored authority by destructive Git commands.
- The production command-spy proof enters through `app.RunTaskUntilTerminal`,
  uses the real workspace/commit composition and ordinary command runner,
  fails on any prohibited verb, and requires actual worktree and commit verbs
  to have been observed. A separately registered linked worktree is snapshotted
  across the operation and must retain its exact branch, HEAD, status, tracked
  bytes, untracked sentinel bytes, and mode.

## EXT-09 Exact Task-Workspace Authority (2026-07-16)

- The deterministic workspace tuple is one indivisible authority: task and
  workspace IDs, canonical control and execution roots, Git common directory,
  task branch ref, ownership-marker path, and exact baseline commit. Reopen and
  commit reconciliation compare that tuple before acquiring the Git-admin
  lock, so a changed control-root relationship cannot create runtime state in
  task source before refusal.
- The ownership marker is harness runtime evidence and therefore uses the
  descriptor-rooted `runtimepath` boundary for directory creation, exclusive
  file creation, opened/named identity recheck, synchronization, and readback.
  A path name, canonical marker-shaped bytes, or a foreign symlink cannot grant
  workspace ownership.
- Git creates the linked-worktree `.git` file and may create its parent
  directories according to the invoking umask. That Git-owned link is read
  through a bounded final-component no-follow descriptor, must be one regular
  single-link inode, and must retain the same named/opened identity across the
  read. Parent symlinks are checked before and after; Git-created directory
  mode is not treated as ownership-marker authority.
- Production proof uses real Git for baseline, ambient-branch, ref, worktree,
  marker, and source evidence. Foreign registration/common-directory results
  are injected only at the ordinary Git command boundary. Every refusal
  compares exact task-workspace HEAD, symbolic branch, porcelain status, and
  tracked bytes before and after, while the positive case proves the ambient
  operator branch never becomes the task workspace.

## EXT-08 Production Attended Terminal Matrix (2026-07-16)

- The terminal production proof enters only through
  `app.RunTaskUntilTerminal`, supplies no task `StepRunner`, and uses the
  separately compiled strict fake through the ordinary production coordinator.
  Separate cases bind needs-input, block, verification failure, no progress,
  safety refusal, cancellation, exact terminal replay, and maximum-cycle
  authority to exact task, state, workspace, Git, receipt, task-run, and ledger
  evidence.
- A supervisor failure is a trusted safety stop only when the cycle carries a
  typed changed `SourceDifference`, proving violation of supervisor read-only
  authority. Safety classification does not inspect error text. Other
  supervisor failures and verification failures remain unsafe or ambiguous.
- Terminal replay re-enters the public app boundary with the exact operation,
  task, configuration, and cycle authority. It performs the required current
  executable admission but starts no additional model process and leaves the
  complete durable task-run operation tree byte-for-byte unchanged.
- Production terminal tests treat absence as evidence: model invocation count,
  verification/receipt artifacts, commit count, canonical task changes,
  state-history categories, finalization artifacts/state/runs/events, and
  task-run ledger transitions are all asserted against a per-case allowlist.

## EXT-07 Production Correction And Re-audit (2026-07-16)

- The correction production proof enters only through
  `app.RunTaskUntilTerminal`, uses the ordinary production coordinator and a
  separately compiled strict fake, and requires one blocking audit, one exact
  cited repair commit, a distinct final verification, exact finding
  resolution, a distinct clean re-audit, and terminal completion. Exact IDs
  and evidence counts make duplicated, skipped, or conflated stages fail the
  proof.
- A top-level auditor dossier may identify the execution state captured before
  attempt admission. Audit application accepts that identity only when it is a
  valid predecessor of the current state and the two states differ solely by
  append-only attempt accounting; lifecycle, plan, workspace, findings, and
  every other authority must remain identical.
- A correction coordinator's mandatory independent re-audit receives an
  ephemeral workspace/state projection at the correction commit and resulting
  source revision. This projection grants read-only audit authority and does
  not advance durable checkpoint state; the durable checkpoint remains gated
  on the full correction sequence succeeding.
- Audit output verification provenance is compared by canonical JSON value,
  because worker-visible JSON intentionally excludes in-memory-only fields.
  Stored auditor profiles reopen under the same strict identity rule used at
  admission: either exact file bytes or deterministic surrounding-whitespace
  normalization must match the recorded digest and size.

## EXT-06 Production-Composition Happy Path (2026-07-16)

- The attended production-composition proof enters only through
  `app.RunTaskUntilTerminal`, supplies no task `StepRunner`, and uses the real
  production workspace, cycle, attempt, optional-role, verification, commit,
  checkpoint, audit, and finalization owners. A separately compiled strict
  fake remains the only model substitute and is invoked by the ordinary
  runner as five fresh ephemeral Codex processes.
- Deterministic production IDs and a test release manifest are injectable only
  through unexported app test seams. The admitted manifest is propagated to
  each Codex invocation so the strict fake exercises the same exact executable
  identity checks; ordinary public callers continue to use the embedded
  release-authored manifest and nondeterministic production IDs.
- A cycle result carries the worker's role-projected dossier rather than the
  supervisor's broader dossier. Audit admission therefore validates the exact
  auditor role dossier against the post-commit safety workspace and separately
  validates the supervisor dossier provenance. Profile evidence may represent
  either exact file bytes or the prompt loader's deterministic surrounding-
  whitespace normalization, but the supplied digest and size must match one
  representation exactly.
- Production step restart authority comes from durable current audit history,
  not only the transient task-run operation projection. Missing latest
  mutation, verification, or audit values are rehydrated before policy
  evaluation. An optional source-changing role's required nested audit receives
  an ephemeral state/workspace projection at the exact committed head while
  durable checkpoint authority remains unchanged until the complete attempt
  succeeds.
- Checkpoint advancement is admitted from the exact verified run-owned commit
  returned by the cycle. Completion live-evidence revalidation may unwrap only
  the matching in-progress finalization envelope to recompute the frozen state
  identity; a different operation/run or any unrelated state drift remains a
  hard refusal.

## EXT-05 Strict Reusable Fake-Codex Contract (2026-07-16)

- The production autonomous integration fixture is a separately compiled Go
  executable under `internal/app/testdata`, not an injected command function,
  in-process Codex shortcut, or replacement task `StepRunner`. App tests invoke
  it only through the ordinary bounded runner used by `codexexec.Run`.
- One strict sibling JSON contract owns version-call and exec-call counts,
  exact invocation order, argv, working directory, prompts, schema paths and
  bytes, full environment identity, last-message bytes, ordered JSONL records,
  and optional receipt bytes. Mutable sibling state contains only completed
  counts and emitted event types, making missing, duplicate, reordered, or
  surplus calls observable and fail-closed.
- Every exec contract must itself be a fresh `exec --json --ephemeral`
  invocation with one final stdin marker and no `resume`. The fake validates
  before writing output and exits with a distinct refusal status on argv,
  directory, schema, prompt, environment, count, or output-sequence drift.
- Complete environment equality is represented without persisting ambient
  values: the contract stores sorted variable names and a SHA-256 over the
  exact sorted name/value entries. The fake compares both projections and
  reports names only on mismatch.
- Deterministic supervisor and worker last-message/JSONL material plus an exact
  worker receipt are caller-supplied fixture evidence. This keeps the reusable
  executable mechanism independent of a particular future happy path while
  allowing later app tests to bind the real production schemas, prompts,
  artifact paths, and output sequence exactly.

## EXT-04 Release-Authored Executable Identity Authority (2026-07-15)

- Release executable authority is an embedded, strict-schema manifest. The
  first manifest contains exactly one build: the exact string `codex-cli
  0.144.4` paired with SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  Semantic ranges, alternate version spellings, unlisted versions, and the
  listed version emitted by different bytes are not authority.
- An executable identity consists of the configured command spelling, the
  canonical absolute symlink-resolved regular-file path, and the lowercase
  SHA-256 of the opened bytes. Codex adds its exact discovered version. Git
  uses the same executable identity without a release version allowlist. The
  path lookup result or version output alone never grants execution authority.
- Mode-aware preflight inspects and renders both identities. Production
  autonomous execution rechecks them before computing the effective
  fingerprint or creating task effects; `codexexec.Run` rechecks the Codex
  identity and current release manifest again before artifact creation and
  executes the admitted resolved path. This makes preflight a snapshot rather
  than a transferable lease and rejects drift at the invocation boundary.
- Effective-config schema v7 includes both identities, and supervisor and
  worker invocation provenance records the same projections. Config check is
  diagnostic: it retains and fingerprints a well-formed observed identity
  even when the current release refuses it, rendering the refusal separately;
  doctor and execution remain fail-closed. Unresolved or malformed identities
  are never admitted.

## EXT-03 Initial External Scope and Attended Bounds (2026-07-15)

- One shared external-scope projection owns the initial platform, Git shape,
  submodule, cleanliness, verification-presence, and operational-bound checks.
  Mode-aware preflight renders that projection and public attended-task,
  queue, and daemon execution re-runs it before acquiring the autonomous lock
  or creating workspace, ledger, task, model, or verification effects.
- Attended-task is admitted only on Linux, macOS, and FreeBSD. Queue and daemon
  are admitted only on Linux. Repository authority requires a resolved
  configured Git executable, a non-bare worktree whose exact top-level is the
  requested repository root, no active recursive submodule, and a clean Git
  worktree. External autonomous admission requires at least one effective
  verification command even if a legacy missing-verification compatibility
  switch is configured.
- Level-1 defaults are 16 task attempts, 4 attempts for each canonical action,
  4 hours elapsed, 1,000,000 model tokens, and 50 cycles per task. Process and
  per-stream output bounds are the largest effective configured timeout and
  cap (30 minutes and 256 KiB by default); retained disk bytes come from the
  retention operation cap (1 GiB by default); enabled notification attempts
  come from the required positive notification policy limit. Unlimited
  attended cycles are not external authority.
- Operational bounds are part of effective-config schema v6 and its
  fingerprint. Config check and doctor render the same canonical projection;
  task-run operation state and immutable ledger events record an exact copy,
  and replay requires it to agree. An omitted config-check work directory is
  normalized to the current directory before descriptor-backed inspection so
  the real CLI retains its established default.

## EXT-02 Mode-Aware Read-Only Preflight (2026-07-15)

- `app.PreflightMode` is a closed request authority: empty normalizes to
  `attended-task`, and the only explicit values are `attended-task`, `queue`,
  and `daemon`. Bare doctor therefore renders byte-for-byte identically to the
  explicit attended form. An exact task selector belongs only to attended
  mode, uses canonical task-ID grammar without trimming, and must currently be
  an autonomous task with shared scheduler readiness `ready`.
- Request-shape validation precedes worktree resolution and every repository
  read, executable lookup, command, or write. Explicit empty CLI flags,
  unsupported modes, malformed task identities, and queue/daemon selectors
  cannot reach preflight effects.
- `loadAutonomousGraph` is the shared current-snapshot loader for mode-aware
  preflight, fresh exact-task admission, and queue/daemon snapshots. It
  strictly loads canonical tasks and every autonomous state/child-publication
  authority, verifies protected archive evidence with the effective command
  runner and read-only ledger, and applies the shared graph validator.
- Preflight records the normalized mode, optional task identity, canonical and
  autonomous task counts, and exact selected-task readiness. Unsafe protected
  state or invalid graph authority stops before the ordinary Codex/Git doctor
  commands. Execution independently invokes the loader again; a successful
  preflight is never a transferable lease.

## EXT-01 Shared Repository-Path Admission (2026-07-15)

- `internal/repositorypath.Inspect` is the common read-only authority check for
  present `.agent`, direct canonical task Markdown, `.revolvr`, config, and
  ledger paths. It binds the canonical repository-root inode, uses no-follow
  descriptor traversal, and requires safe directory modes plus single-link,
  safe-mode regular files. Missing paths are presence facts and inspection
  never creates directories, files, locks, SQLite sidecars, or other evidence.
- Status initialization retains its compatibility meaning: `.revolvr` and its
  ledger are present. Missing `.agent` remains a safe empty canonical-task
  namespace, while any present unsafe `.agent` or `.revolvr` authority is a
  refusal shared by doctor, status, task loading, and autonomous admission.
- Canonical task enumeration and task/config reads reuse the inspected
  descriptor-root identity and recheck named/opened identities around reads.
  Existing task containment diagnostics remain an earlier compatibility check;
  they cannot admit anything rejected by the common boundary.
- Public exact-task and queue operations inspect before acquiring the global
  autonomous-execution flock. This ordering is required so an unsafe no-model
  admission probe cannot create a lock namespace or any other runtime effect.
  Execution and later owners still recheck their narrower authority; preflight
  remains a current snapshot rather than a transferable lease.

## External Autonomous Readiness Policy (2026-07-15)

- External-project readiness is approved in order: attended single task,
  unattended bounded queue, then unattended daemon. The first queue approval
  is sequential with one worker; parallelism is a later approval rather than a
  prerequisite for safe external use.
- An unprovable in-flight model or command boundary is never heuristically
  resumed. Exact idempotent transitions still replay, while the affected task
  is durably quarantined with immutable `unsafe_or_ambiguous` operation
  evidence. Unrelated queue work may continue only after durable exclusion.
  Recovery is an explicit new operator action that reconciles every authority
  and creates a new operation identity; generic retry does not erase the old
  occurrence.
- The initial unattended deployment profile is Linux in a rootless OCI
  container with only the project/control root writable, read-only container
  root, private/bounded process and temporary resources, no host home or
  privileged sockets/devices/capabilities, disabled Git hooks, replacement
  environment, redacted declared Codex credentials, and externally enforced
  default-deny egress limited to the tested Codex endpoint set. Arbitrary task
  network access is not initially supported.
- Mode-aware `doctor --for attended-task|queue|daemon` is the preflight
  contract. Bare doctor remains the attended compatibility form. Preflight,
  status, and no-model admission share local authority checks; execution
  always rechecks and never treats preflight as a lease.
- External autonomous admission uses a release-authored allowlist of exact
  Codex version strings and resolved executable SHA-256 identities. The first
  release may support one exact CLI build. Go 1.22 remains the language floor,
  while release binaries use and record a currently supported patched Go
  toolchain with no reachable vulnerability.
- Unattended operation requires explicit finite positive attempt, action,
  elapsed, token, cycle, queue-task, daemon-sweep, process, output, disk, and
  notification bounds. Unlimited token or time authority is invalid. Every
  bound is fingerprinted, rendered, and recorded.
- The first supported repositories are operator-controlled non-bare Git
  repositories without active submodules. Revolvr never pushes, merges,
  rebases, resets, cleans, or stashes them, and never archives completion
  automatically. Review and integration remain explicit operator actions.
- Soak gates are quantitative. Level 1 requires 10 real task operations across
  two external repositories and the named success/failure/input/cancel/safety
  scenarios. Level 2 requires three sequential queues totaling 20 operations
  plus dependency, yield, cancellation, restart, and quarantine continuation.
  Level 3 requires 72 continuous hours, 10 external wakes, two clean restarts,
  and one interrupted active sweep. Containment, duplication, evidence-loss,
  manual-state-edit, or unclassified-ambiguity failures invalidate approval.
- The complete task-authoring source is
  `.agent/AUTONOMOUS_EXTERNAL_READINESS.md`. New backlog decomposition must
  preserve these decisions and create small independently verifiable tasks;
  policy is reopened only for direct contradictory evidence.

## AUDIT-R4-CLOSE-01 Final Audit Closure (2026-07-14)

- Closure is requirement-by-requirement, not inferred from implementation
  commits or a green broad suite. Current source must expose each promised
  authority, every named owner and reader must use it, and fresh focused
  adversarial tests must prove each reproduced failure boundary before the
  resolved audit document is removed.
- `AUDIT_PROBLEMS.md` is an active finding list rather than permanent history.
  Once AP-01 through AP-06 are independently proven closed and the complete
  audit matrix passes, the file is deleted. Git history plus `.agent/STATE.md`
  and `.agent/DECISIONS.md` retain the findings, corrections, and proof.
- CLI verification uses a freshly initialized Git fixture with `umask 0022`.
  A developer working copy whose pre-existing `.agent` or `.git` directory is
  group-writable is expected to fail the filesystem safety boundary; closure
  does not weaken that boundary or mutate the developer's local permissions.
- The final platform matrix includes the original audit architectures and the
  repository's current CI targets, plus Darwin/FreeBSD compilation of the
  platform-specific source-snapshot tests and the Windows diagnostic stub.

## AUDIT-R4-11 Deterministic Residual Diagnostics (2026-07-14)

- When several action-budget authorities regress in one transition, action
  string order is the diagnostic precedence. Transition validation sorts a
  copy of the prior budget slice and never lets a map traversal choose the
  reported action.
- Archive commit paths are the canonical order for expected byte checks. The
  same sorted path slice first proves the exact committed path set and then
  selects each expected payload for object read and comparison; entries that
  intentionally have no byte payload remain exact path-only evidence.
- Determinism regressions are multi-invalid by construction and assert exact
  errors repeatedly. They also assert archive object-read order so stable text
  cannot mask nondeterministic command execution.

## AUDIT-R4-10 Descriptor-Bound Source Snapshot Identity (2026-07-14)

- Source-entry evidence belongs to the opened filesystem object, not merely
  to matching pathname observations. Regular capture therefore uses a
  no-follow, nonblocking descriptor and requires its immediate `fstat`
  identity, type, mode, size, and modification time to match the initial
  `Lstat`.
- A regular digest is accepted only after a second descriptor check proves
  the opened object remained stable and a final `Lstat` proves the name still
  designates that object. Reads whose byte count disagrees with descriptor
  size fail closed.
- Symlink targets require equivalent descriptor authority. Linux opens the
  link with `O_PATH|O_NOFOLLOW` and uses empty-path `readlinkat`; macOS opens
  it with `O_SYMLINK` and uses `freadlink`; FreeBSD uses its
  `O_PATH|O_NOFOLLOW` descriptor and empty-path `readlinkat`. A platform or
  filesystem that cannot provide the primitive fails capture instead of
  falling back to a pathname `Readlink` race.
- Symlink target reads are dynamically sized with an explicit upper bound and
  truncation detection. The initial path, opened descriptor before/after the
  read, target length, and final path must all agree before target bytes are
  hashed.
- Test-only capture points bracket initial lookup, descriptor open, reads,
  and descriptor/path rechecks. ABA regressions equalize mode, size, and
  modification time so only inode binding can satisfy the contract.

## AUDIT-R4-09 Codex Usage Schema Authority (2026-07-14)

- Usage authority follows one explicit precedence sequence: top-level
  `usage`, `total_usage`, `token_usage`, and `total_token_usage`; those keys
  under `response`, `result`, `message`, and `event` in parent/key order; then
  metric fields on the event itself.
- Arbitrary-depth named usage objects are a legacy compatibility schema, not
  an ordered search. The parser enumerates metric-bearing candidates and may
  use the recursive shape only when exactly one exists. Multiple candidates
  fail closed; sorting exists only to make their JSON-pointer paths stable and
  never supplies selection authority.
- `CodexUsageAmbiguityError` records the JSONL record and candidate paths and
  unwraps to `ErrCodexUsageAmbiguity`. It is an invalid/incomplete optional-
  metrics diagnostic rather than an artifact read failure. Callers publish no
  partial usage, and receipt rewriting preserves the original receipt.

## AUDIT-R4-08 TUI Quit-After-Settlement (2026-07-14)

- TUI quit is a requested terminal transition while an operation is active,
  not immediate program termination. `q` and `ctrl+c` mark the current
  token-scoped run for quit, request cancellation, and keep the existing
  Bubble Tea command/message stream alive.
- The domain operation's exact terminal message is the join boundary. It is
  published only after run/loop/task/queue execution and status refresh (plus
  run-detail refresh where applicable) finish. The model applies that message
  completely before emitting `tea.Quit`.
- A delayed quit is released only by the current run token. Stale progress or
  terminal messages cannot stop the program, clear active state, or transfer
  quit authority to another operation.
- Quit-pending does not start a second waiter or close the event channel. The
  outstanding start/wait command continues consuming the buffered stream;
  cancellation-aware progress callbacks stop enqueueing, and the terminal
  publication remains drainable without a send/quit deadlock.
- For autonomous task and queue modes, terminal application followed by quit
  supersedes the ordinary selector-reload command. Status refresh and durable
  cleanup are already part of terminal construction, so no domain settlement
  is skipped.

## AUDIT-R4-07 Bounded Post-Kill Process Settlement (2026-07-14)

- Graceful termination and forced-kill settlement are separate lifecycle
  phases with separate deadlines. `TerminateGracePeriod` bounds TERM handling;
  `KillSettlementPeriod` bounds proof that the group is gone after KILL and
  defaults independently to five seconds.
- A successful `SIGKILL` syscall is not settlement evidence. The runner polls
  the process group until absence, and inability to prove absence before the
  kill-settlement deadline is `ErrProcessTreeUnsettled` joined with the
  original cancellation/deadline and all retained signal/inspection errors.
- Signaling and liveness inspection use one guarded process-tree lifecycle.
  While the command leader is unreaped its PID remains the operation's
  identity. Once `cmd.Wait` completes, every later signal or poll first applies
  the exited-leader reuse check; a reused identity stops settlement without a
  platform signal or liveness syscall against the replacement.
- Natural leader exit and cancellation deliberately share that guard. A race
  in which cancellation signals before reap but polls after reap therefore
  transitions to strict exited-leader identity checks rather than retaining
  stale pre-reap authority.
- Deterministic settlement testing uses both an injected lifecycle and a real
  Linux process group. The real regression keeps a force-killed, TERM-ignoring
  child as an unreaped zombie so the group remains observable, proving `Run`
  waits for group disappearance and that the no-mutation oracle begins at its
  return boundary.

## AUDIT-R4-06 Stable Remaining Evidence Readers (2026-07-14)

- Capped protected reads belong in `runtimepath`, not in pathname prechecks at
  each owner. `Boundary`, `Directory`, and `File` expose one limit-preserving
  read path that checks the opened size before allocation, bounds the stream,
  and revalidates the named/opened identity after reading.
- An operation that consumes multiple related artifacts retains one
  `runtimepath.Boundary`. Audit/plan application, operator receipts, ledger
  export verification/replay, autonomous views, and migration inspection do
  not independently resolve the root between authoritative reads.
- A retained `File` separates stable unaliased inode identity from safe-mode
  policy. Ordinary reads/writes still require both. `Perm`, `Chmod`, and
  descriptor-relative cleanup may inspect or restrict an externally changed
  mode only while the parent/name/inode still match and link count remains one;
  aliases and namespace substitutions remain errors.
- Codex last-message raw output is external-writer input. The harness opens it
  relative to the retained parent only after child completion, validates its
  owner-only mode, and then keeps that file and the redacted temporary open
  through read, publication, cleanup, readback, and sync. A completed replace
  is recorded before post-publication checks so cleanup never removes the
  canonical artifact.
- The AP-01 inventory is defined by its named authoritative readers and exact
  occurrences of their `Lstat`-then-by-name shape. The directly discovered
  autonomous-migration orphan-state reader is included in this migration;
  source-snapshot hashing remains the separately bounded `AUDIT-R4-10` task.

## AUDIT-R4-05 Stable Finalization Artifact Store (2026-07-14)

- One finalization transaction owns one descriptor-rooted artifact store for
  frozen evidence, capsule, and manifest publication and replay. Canonical
  task and state mutation stay with `taskfile` and `autonomousstate`; this
  store does not duplicate or weaken those owners' locking/CAS contracts.
- Completion artifacts are immutable. Publication uses an opened, synced
  temporary inode and descriptor-relative exclusive link rather than an
  overwrite-capable rename. An exact existing artifact is replay; a
  conflicting, aliased, unsafe-mode, or substituted destination is an error.
- The store retains the opened parent and temporary identity through
  publication, directory sync, and exact descriptor readback. It records a
  completed link before post-publication checks, distinguishing a durable
  final name from an unpublished temporary even when later validation fails.
- Cleanup unlinks only the still-opened unpublished temporary relative to its
  retained parent. If namespace authority is lost, leaving the temporary in
  the displaced harness directory is preferable to following a replacement
  pathname and mutating an attacker-selected outside entry.
- Replay and verification read through protected file descriptors and require
  exact path/hash/size/bytes. The finalization-specific parent walker and all
  check-then-reopen publication, cleanup, sync, and readback helpers are
  removed.
- Deterministic fault points cover before/after directory open, temporary and
  read open, file sync, pre/post publication, directory sync, readback, and
  cleanup. The permanent end-to-end oracle includes transaction state and the
  complete outside-tree contents, modes, symlink targets, and link counts.

## AUDIT-R4-04 Stable Autonomous Archive Store (2026-07-14)

- One archive or reopen transaction owns one descriptor-rooted
  `archiveStorage` and retains the actual Git-admin and archived-task state
  flocks for the transaction. Read-only list/show/load/verify operations own
  the same stable root authority without inventing a mutation lease.
- Immutable archive artifacts and transition history publish an opened,
  synced temporary inode through a descriptor-relative exclusive link.
  Mutable journals use descriptor-relative replacement. Both paths retain the
  opened file through stable-directory sync and strict readback, and record a
  completed publication before post-syscall checks so cleanup cannot remove a
  final artifact.
- Exact active-task removal opens and validates the expected artifact, then
  unlinks that inode relative to its retained parent. Cleanup likewise removes
  only an unpublished opened temporary while the directory and every held
  lease still validate; lost authority deliberately leaves local residue.
- Archive hierarchy enumeration recursively opens child directories relative
  to their retained parents and reads allowed files through protected file
  descriptors. Manifest/state/frozen/journal/reopen/history reads share this
  boundary, replacing the archive-specific symlink walker and all check-then-
  reopen pathname helpers.
- Reopen performs its locked revalidation through the already leased store,
  re-reads the selected manifest and completed journal, and requires the same
  archive/task identities observed before lock admission. This prevents a
  valid but substituted archive selection from becoming mutation authority.
- Deterministic fault points cover directory, temporary, and read opens;
  enumeration; file and directory sync; pre/post publication; readback;
  cleanup; and pre/post removal. The permanent matrix treats outside-tree
  contents, modes, symlink targets, and link counts as the no-mutation oracle.

## AUDIT-R4-03 Stable Exact-Task-Run Store (2026-07-14)

- One exact-task run owns one descriptor-rooted store and one exclusive
  operation `Flock` for its complete lifecycle. Admission, cycle, recovery,
  and terminal persistence share the retained operation/history directories;
  the lease remains available for identity checks rather than being converted
  to a close-only function.
- Immutable operation history is published from an opened, synced temporary
  inode with a descriptor-relative exclusive link. Mutable `operation.json`
  is published from an opened temporary with descriptor-relative replacement,
  followed by stable directory sync and exact-byte protected readback.
- Recovery reads checkpoint and history bytes through protected file
  descriptors and enumerates the retained history directory. History remains
  authoritative when newer than the checkpoint; this task changes filesystem
  identity ownership, not the established operation-stage recovery policy.
- Every mutation and authority-acceptance boundary checks the retained
  namespace and original lease. Cleanup removes only the still-opened
  temporary from its stable parent; namespace or lease loss deliberately
  leaves harness-owned residue in the displaced directory.
- The package-level `persist` helper remains only for tests and durable setup;
  it acquires the same hardened lease and store for its single transition.
  Production never reacquires between transitions. Deterministic fault points
  bracket both opens, file/directory syncs, immutable link, mutable replacement,
  readback, and cleanup so ancestor and lease substitutions stay testable.

## AUDIT-R4-02 Stable Notification Persistence Boundary (2026-07-14)

- One notification delivery owns one descriptor-rooted store for its complete
  operation. The store retains the stable delivery directory and the actual
  exclusive `Flock`; a closure that can only unlock is insufficient because
  persistence must prove the named lease inode immediately before metadata
  mutation and before accepting authority.
- Immutable notification intent, payload, and history use opened temporary
  inodes plus descriptor-relative exclusive link publication. Mutable
  `journal.json` uses an opened temporary plus descriptor-relative atomic
  replacement. The opened file records its published name before post-syscall
  validation so cleanup never mistakes a completed publication for an
  unpublished temporary.
- The runtime-path boundary supports direct child-directory open and creation
  from a retained parent. Notification history and list traversal use that
  operation so a transaction does not re-resolve child authority through a
  substituted ancestor pathname.
- Protected reads and enumeration retain the same delivery/history handles
  through journal reconstruction, intent/payload identity checks, and list
  projection. Notification-local `Lstat`/read walkers, by-name publication,
  rename, removal, and sync helpers are removed.
- Cleanup requires both the stable namespace and original lease. If either is
  lost, leaving a harness-owned temporary in the displaced directory is safer
  than risking removal of an attacker-selected entry. Fault seams bracket
  open, write/sync, publication, post-publication, directory sync, and cleanup
  so this rule remains permanently testable.

## AUDIT-R4-01 Descriptor-Rooted Runtime State Boundary (2026-07-14)

- `runtimepath.Boundary` is a value authority over one canonical repository
  root device/inode. It retains no file descriptor; each operation opens the
  named root without following the final component and requires that original
  identity before traversing with stable directory descriptors.
- Existing and created descendants are opened relative to their validated
  parents with no-follow operations. `Directory` and `File` handles retain
  opened identities across create, enumerate, read, exclusive link
  publication, replacement rename, unlink, file/directory sync, and readback.
  Named/opened identity and safe type/mode/link-count checks bracket every
  operation that returns or mutates protected evidence.
- The legacy string-based runtime-path API delegates to this boundary so
  current owners gain descriptor-rooted single-operation safety without a
  flag day. Durable owners that require one root or parent identity across a
  transaction retain the `Boundary` or opened `Directory` directly as they
  migrate in AUDIT-R4-01 through AUDIT-R4-06.
- `autonomousstate.Store` binds one root identity at construction. Canonical
  state and all transition history reads use protected descriptors; immutable
  evidence and state temporaries are created relative to stable parents; and
  the same opened task directory and temporary inode own CAS read, rename,
  sync, and readback. The held state flock is checked immediately before state
  metadata replacement and before readback acceptance, and around immutable
  publication.
- Cleanup is fail-closed. It unlinks only the still-opened file from its stable
  named parent while the state lease remains valid. Namespace or lease loss
  leaves inspectable harness residue rather than risking an attacker-selected
  unlink.
- The audit reproducer is permanent. Replacing the task namespace with an
  outside symlink at `FailureBeforeStateRename` cannot create outside
  `state.json`, consume the attacker temporary, or change outside contents,
  entries, permissions, or hard-link counts.

## AUDIT-R3-CLOSE-01 Final Audit Closure (2026-07-14)

- `AUDIT_PROBLEMS.md` is deleted only after current source inspection and fresh
  focused executions independently prove AP-01 through AP-08, followed by the
  ordinary, original-seed shuffled, race, vet, module, smoke, CLI, shell, and
  cross-platform matrices.
- Fake-Codex smoke repositories explicitly set `umask 0022`. Their Git admin
  directories must satisfy the same no-group-write initialization boundary as
  production repositories regardless of the invoking developer's host umask;
  the smoke fixture adapts to that contract rather than weakening it.

## AUDIT-R3-08 No-Caller Surface Removal (2026-07-14)

- Private wrappers with no caller are not retained as speculative convenience
  APIs. Notification persistence keeps the fault-aware primitives as the sole
  implementation path, with ordinary production calls passing `nil` through
  the existing admission and transition wrappers.
- The uncalled admitted-cycle composition is not a reserved contract. It is
  internal-only, had no test or documented consumer, and duplicated the
  admission/completion orchestration now owned by the production app path, so
  its config, result, and runner were deleted together.

## AUDIT-R3-07 Deterministic Validation Diagnostics (2026-07-14)

- Validation error precedence is part of Revolvr's deterministic contract.
  Fixed fields are checked in an explicit slice rather than a map: source
  snapshot hashes use index, worktree, snapshot order, and dossier-cache root
  identities use control, execution order.
- When more than one open audit finding disappears, `ApplyReport` collects and
  lexically sorts all missing IDs before selecting the diagnostic. This keeps
  lookup maps internal while preventing their undefined iteration order from
  choosing externally visible behavior.

## AUDIT-R3-06 Component-Aware Repository Paths (2026-07-14)

- A clean repository-relative component beginning with `..` is not traversal.
  Dossier-cache guidance and Git-tree validation reject only exact `..` or a
  cleaned path beginning with `..` plus the platform separator.
- Absolute and noncanonical paths remain invalid before publication; Git-tree
  validation also retains its UTF-8/NUL, duplicate, type/mode, and protected
  runtime-path rules. This change does not reinterpret backslashes as
  separators on the supported Unix platforms.
- Real-Git assembly coverage commits `..foo` and `..well-known/file` and proves
  those exact names remain visible in the deterministic repository-map dossier.

## AUDIT-R3-05 SHA-1 and SHA-256 Git Object Identities (2026-07-14)

- `internal/gitoid.Valid` is the single full-Git-object-ID validator. Revolvr
  supports both lowercase SHA-1 (40 hexadecimal characters) and lowercase
  SHA-256 (64 hexadecimal characters); abbreviated, uppercase, non-hex, and
  padded forms are invalid.
- Existing workspace, state, workspace-manager, archive, finalization, and
  dossier-cache validators delegate to the shared grammar. `git ls-tree -z`
  parsing and repository-map dossier projection apply the same check rather
  than imposing a delayed SHA-1-only length rule.
- SHA-256 support is behavioral rather than a schema claim alone: when the
  installed Git supports `--object-format=sha256`, an end-to-end assembly test
  creates a real repository and proves map parsing, cache publication/hit, and
  dossier projection using its 64-character commit, tree, and blob identities.

## AUDIT-R3-04 Fence-Aware Markdown Structure (2026-07-14)

- `internal/markdown.Fence` owns the small shared fence grammar used by task
  import and receipt structure. Backtick and tilde openers accept zero through
  three leading spaces; only a same-marker, whitespace-only closer at least as
  long as the opener ends the fence. An unclosed fence remains active to EOF.
- Structural task and receipt headings are recognized only on lines classified
  outside a fence. Scanning does not rewrite the line, so fence boundaries and
  contents stay in imported tasks and receipt bodies byte-for-byte within the
  existing line-ending behavior.
- Receipt required-section discovery, claim-section selection, and harness
  body rewriting share that classification. Claim parsers still accept claims
  inside code blocks belonging to a real Changed Files or Verification
  section, but a fenced heading cannot start, end, or redirect such a section.

## AUDIT-R3-03 Live-Reader Busy Evidence Retention (2026-07-14)

- `retryLiveRead` retains the most recent SQLite busy/locked error across the
  complete retry operation. A later cancellation/deadline operation result or
  observed context termination returns the result type's zero value and joins
  that context cause with the retained SQLite evidence.
- Busy evidence is causal, not ambient: no preceding busy attempt means no
  SQLite error is attached, and a later ordinary non-context failure returns
  normally. When the bounded busy retry window itself expires, the latest busy
  error remains the result.
- The rollback-journal regression uses a real SQLite busy error but scripts the
  retry sequence rather than depending on a 50ms deadline race. It proves
  deadline and cancellation cause discovery, newest-error retention, zero
  results, absence of invented busy evidence, and healthy reads through both
  the existing and a reopened live reader after rollback.

## AUDIT-R3-02 Initialization Filesystem Trust Boundary (2026-07-14)

- Initialization resolves one canonical worktree identity and completes a
  read-only validation plan before creating state, agent, profile, task, or
  ledger material. Every existing protected component must be a no-follow,
  repository-contained directory or single-link regular file with no
  group/other write permission.
- `runtimepath.OpenFile` is the writable protected-file entry point. It refuses
  truncation before validation, opens with `O_NOFOLLOW`, and proves the opened
  descriptor still names the validated file. Callers revalidate after their
  write; replacement can invalidate the operation but cannot redirect the
  opened descriptor to an outside target.
- Init securely creates the ledger file before SQLite initialization and keeps
  an identity-checked descriptor open across the writable ledger open/close.
  Profile creation is exclusive and never overwrites an existing validated
  profile. Task creation validates/creates `.agent/tasks` through the same
  protected directory boundary and exclusively creates the final task file.
- A repository-controlled `.git` file is not itself Git-admin authority.
  Bounded Git queries provide the top-level, per-worktree admin directory,
  common directory, and effective `info/exclude`; all four are cross-checked.
  An external admin directory is accepted only for Git's real linked-worktree
  shape and reciprocal `.git` backlink. Normal repositories update
  `<worktree>/.git/info/exclude`; linked worktrees update the common exclude.
- Git exclude content is read and appended through one opened descriptor with
  identity checks before read, before write, and after sync. Symlink, hard-link,
  unsafe-mode, wrong-type, and opened-path substitution cases fail without
  writing the attacker-selected target.

## AUDIT-R3-01 Natural-Exit Process-Group Settlement (2026-07-14)

- A direct command's exit is not the process-tree completion boundary.
  `runner.Run` inspects the dedicated process group after `cmd.Wait` and joins
  the same bounded TERM/poll/KILL settlement used for cancellation whenever a
  group member remains.
- `ErrProcessTreeUnsettled` is a runner-level lifecycle failure. It is returned
  even when the direct child exited zero, while the direct exit code remains
  available as evidence. Cancellation and deadline causes remain authoritative
  when they race with or trigger settlement.
- Natural-exit signalling is permitted only while the reaped leader PID has
  not been occupied by another process. PID reuse makes the boundary fail
  closed without signalling the possibly unrelated process-group identity.
- The boundary covers ordinary descendants that remain in the inherited
  process group; it does not change the established non-sandbox contract for a
  hostile child that deliberately escapes that group.

## AUDIT-FIX-06 Supported Platform Contract (2026-07-14)

- Revolvr's operational CLI supports Linux, macOS, and FreeBSD. These are the
  platforms whose Unix process-group, advisory-flock, no-follow-open, inode,
  link-count, and directory-sync semantics the current safety model relies on.
- `cmd/revolvr/main.go` is compiled only for those three operating systems.
  Other operating systems compile a dependency-free diagnostic command that
  names the unsupported platform and exits unsuccessfully before importing or
  running any Revolvr workflow package.
- GitHub Actions runs the full suite on Linux and cross-builds the CLI for
  Linux, Darwin, and FreeBSD on amd64. A separate Windows cross-build and
  message check preserves the intentional unsupported-platform stub without
  representing Windows as an operationally supported target.

## AUDIT-FIX-05 App Live Ledger Read Boundary (2026-07-14)

- `internal/app.openReadOnlyLedger` is the sole production app boundary for
  opening raw live ledger evidence. Status, run display, receipt validation,
  task scheduling, archive verification, metrics, and autonomous archive
  scheduling all use it; mutation orchestration retains an explicit writable
  opener.
- The boundary delegates only to `ledger.OpenLiveReadOnly`, preserving ordinary
  live SQLite locking and query-only behavior without directory creation,
  schema initialization, or migration. Empty, old-schema, and malformed live
  ledgers remain diagnostic evidence rather than implicit recovery requests.
- App integration tests compare the state-directory entry set and the database,
  rollback-journal, WAL, and shared-memory existence, mode, size, modification
  time, and SHA-256 identity around each audited projection. A valid ledger and
  its parent are permission-read-only; invalid fixtures must fail without any
  filesystem mutation.

## AUDIT-FIX-04 Complete Coordinator Flock Migration (2026-07-14)

- `internal/lock.AcquireFlock` is now the only production boundary that invokes
  advisory flock. Autonomous execution, archive Git/state coordination, child
  publication, autonomous migration, notification delivery, queue and task-run
  operations, autonomous state compare-and-swap, workspace Git administration,
  and artifact-GC inner admission probes all use it. Direct `syscall.Flock`
  remains only in `internal/lock/flock_unix.go`.
- A nonwaiting would-block result is wrapped with `ErrFlockContended` while
  retaining the platform error. Callers map only that sentinel to their public
  active-owner result; unsafe paths and other acquisition failures remain
  visible instead of being mislabeled as contention.
- `FlockConfig.AfterOpen` is the deterministic fault/substitution seam shared by
  owner integrations. The primitive always validates the opened descriptor
  before the seam and validates named/opened identity again after flock, so a
  rename and replacement during that window cannot establish authority.
- `FlockConfig.PollIntervals` defines a finite retry sequence whose last value
  repeats. Autonomous state uses it to retain the audited immediate acquire and
  1/2/4/8/16/20ms capped retry contract; its store helper also checks context
  before acquisition so a pre-canceled transition creates no runtime state.
- Notification delivery passes its actual repository root into the lock owner;
  it no longer infers a root from a nested delivery directory. Git-admin locks
  now use the shared restrictive `0700` directory and `0600` file defaults.
- Artifact GC holds shared-primitive handles for all four admission barriers,
  rechecks inner identities with the retention identity, and reports an active
  coordinator only for `ErrFlockContended`.
- Shared and integration regressions prove final-component symlink rejection
  without sentinel mutation, hard-link rejection, symlinked-ancestor rejection,
  and open-to-flock path substitution for Git administration and delivery in
  addition to the existing source-writer, retention, child, and queue paths.
  Path-test link counts explicitly convert platform `Stat_t.Nlink` widths so
  the same suites compile on Darwin and FreeBSD.

## AUDIT-FIX-03 Hardened Flock Foundation (2026-07-14)

- `internal/lock.AcquireFlock` is the shared predictable-lock acquisition
  boundary. It resolves one canonical repository/control root, accepts only a
  contained relative lock path, creates missing ancestors one component at a
  time through `runtimepath.EnsureDir`, validates an existing final component,
  and creates lock files at `0600` by default.
- No-follow open and advisory flock operations live in build-tagged platform
  files. The common boundary validates the opened regular file, safe mode,
  single-link count, and named/opened inode before flock and again after a
  successful shared or exclusive acquisition. Waiting is bounded by caller
  cancellation; nonwaiting contention preserves its underlying error.
- A held `Flock.Check` is the required identity revalidation immediately before
  a destructive metadata operation. Deterministic tests substitute the named
  path between open and flock and after acquisition; neither case grants safe
  authority over the replacement.
- Source-writer metadata, workspace source-writer metadata, and the shared
  artifact-retention admission gate now use the shared primitive below
  canonical roots. A `SourceWriter` additionally binds the initial source-lock
  inode for its complete logical lease and compares every heartbeat/release
  descriptor to it before reads and truncating writes. Byte-identical metadata
  on a replacement inode cannot inherit ownership.
- Artifact GC takes the same hardened retention inode exclusively, checks it
  after inner coordinator admission and source-writer scans, and checks again
  immediately before export, compression/prune, journal, cancellation, and
  cleanup mutations. Source writers likewise recheck their held shared
  retention inode before source metadata writes.
- This task migrates only the source-writer and outer retention identities.
  Autonomous execution, Git administration, child publication, notification,
  workspace, migration, queue, and other predictable coordinator owners remain
  the bounded `AUDIT-FIX-04` migration; their behavior is not represented as
  hardened merely because retention probes some of their filenames.

## AUDIT-FIX-02 Clean Mixed-Pass Commit Contract (2026-07-14)

- A source-changing pass requires a clean pre-run worktree. The legacy
  `commit.allow_pre_existing_dirty` YAML key remains parseable only for a
  precise migration error: explicit `false` is accepted, while `true` is
  rejected before source locking, Codex, verification, staging, or commit work.
  Direct run-once configuration receives the same pre-side-effect rejection.
- `internal/commit` is the final staging boundary and exposes no bypass. Any
  path in the supplied pre-run capture refuses the commit. Mixed-pass and
  autonomous callers apply the same invariant before source-changing work.
- Filename subtraction and whole-file comparison cannot prove byte ownership
  when operator and run edits overlap. Revolvr therefore does not infer an
  owned delta from a dirty worktree; unrelated and overlapping edits both fail
  closed unless a future design introduces exact isolation authority.
- Real-Git regression coverage captures operator edits, applies simulated run
  and task-metadata edits, and invokes the actual commit boundary. Refusal must
  leave HEAD and the index unchanged and preserve every resulting worktree byte.
- The effective-run projection retains the existing false-valued field and
  schema for valid configurations. Removing an unsafe true setting therefore
  does not churn effective identities for operators already using the required
  clean-worktree contract.

## AUDIT-FIX-01 Source-Lease Terminal Settlement (2026-07-14)

- `lock.SourceGuard.Settle` is the shared terminal ownership boundary. It stops
  and joins the asynchronous heartbeat monitor without releasing the lease, so
  an in-flight persistence or ownership failure is published before any task,
  receipt, ledger, or externally consumed outcome classification is finalized.
- Cancellation-only heartbeat shutdown is not ownership loss, including a
  wrapped cancellation error. A joined independent heartbeat error remains
  ownership loss and is retained alongside the cancellation cause.
- Terminal code settles first and, while the operation remains active, performs
  one final synchronous ownership check. Nonterminal checks settle only when
  cancellation requires joining a possibly in-flight heartbeat; healthy active
  work retains periodic monitoring.
- Mixed-pass ownership failure never transitions the canonical task. It writes
  `safety_limit` receipt evidence and matching failed-run ledger evidence under
  an independent bounded context. Autonomous worker finalization applies the
  same settlement rule before receipt and ledger completion, and a failure
  first discovered during deferred close replaces any unsafe success outcome.
- `SourceGuard.Close` retains settlement and release errors across calls and
  invokes lease release exactly once. Normal execution keeps the lease until
  terminal persistence is complete, then the outer defer releases it.

## AM-02 Autonomous Migration Transaction Authority (2026-07-14)

- `internal/autonomousmigration.Apply` is the sole migration publication and
  recovery boundary. The AM-01 plan material deterministically names an
  operation; immutable source/projected/state artifacts and a contiguous
  four-record history are durable authority, while the mutable journal is only
  a checkpoint that may be missing or behind history but never ahead or
  divergent.
- Publication order is a safety invariant: every batch state is published and
  identity-checked before any canonical task becomes `autonomous-v1`. Task
  publication is an exact compare-and-swap from the AM-01 source bytes to its
  deterministic projection. A crash may expose a partial batch, but never an
  autonomous task pointing at absent state, and replay completes that exact
  batch rather than replanning changed evidence.
- Exact existing bytes are adoptable; different bytes are user/conflict
  authority and are never overwritten. Apply planning alone may admit a task
  namespace containing exactly the canonical initial state and nothing else,
  which recovers an identical orphan without weakening ordinary AM-01 dry-run
  eligibility. Completed operation replay validates current autonomous task
  and state authority but permits legitimate status/lifecycle evolution.
- Application holds the non-waiting outer autonomous-execution lease, the
  source-writer lease, and a cancellable migration flock. It validates the
  shared graph and complete requested batch under those locks, uses only a
  read-only ledger for archive scheduling evidence, and creates no Codex run,
  ledger event, Git commit, or inferred plan/acceptance/finding evidence.
- `revolvr task migrate` applies unless `--dry-run` is explicit. Recovery is
  selected before fresh planning so mixed/autonomous partial publication is
  resumable. Explicit IDs must match one exact durable task set; `--all`
  prioritizes incomplete authority and treats a completed batch as replay only
  when no new active mixed-pass work exists.
- `revolvr init` remains initialization-only. It never migrates tasks or
  creates autonomous task state; strict missing-state admission diagnostics
  direct operators to the migration command for interrupted conversions.

## AM-01 Autonomous Migration Planning Authority (2026-07-14)

- `internal/autonomousmigration.Build` is the deterministic, read-only batch
  planning authority. It consumes one already assembled `taskschedule.Snapshot`
  so strict canonical loading, archive/checkpoint reconciliation, graph
  validation, and migration projection all refer to the same repository
  evidence. Any shared invalid-graph diagnostic rejects the complete batch.
- `--all` means every active `mixed-pass-v1` task, not already-autonomous tasks
  or operator checkpoints. Explicit IDs and all-mode are mutually exclusive;
  selection and rejection output are source-path/task-ID deterministic and no
  candidate is returned when any requested task is ineligible.
- Initial eligibility is deliberately narrow: pending implement-phase mixed
  tasks only, with no child lineage, existing autonomous state/task namespace,
  or unsafe path component. Other phases are unrepresentable prior-phase
  evidence rather than something migration may translate or discard.
- `taskfile.ProjectAutonomousMigration` owns exact task-byte projection. It
  preserves unrelated frontmatter, safe extensions, line endings, and the full
  body while removing mixed-only `phase`/`profile`, setting `workflow` to
  `autonomous-v1`, and adding only the canonical task-specific state path.
- A planned initial state is the current validated AW-02 schema at pending
  lifecycle with retry/time/token budget modes unset and no model-authored or
  inferred evidence. Exact source, projected-task, and state byte identities
  are retained in each plan entry for AM-02 compare-and-publish authority.
- AM-01 CLI execution is always plan-only, including when `--dry-run` is
  omitted. It performs no locks, journals, state/task writes, Codex invocation,
  or Git commit. AM-02 exclusively owns locked application, durable recovery,
  orphan adoption, conflict handling, and replay semantics.

## OC-02 Checkpoint Fulfillment Authority (2026-07-14)

- `internal/app.FulfillCheckpoint` owns operator-facing orchestration: exact
  task/path/operator validation, strict receipt loading, shared graph
  prevalidation, publication request, and post-publication scheduler
  re-evaluation. It has no Codex, ledger-event, or Git-commit side effect.
- `internal/taskfile.FulfillOperatorCheckpoint` is the sole publication
  boundary. Starting from exact canonical task bytes, it changes pending to
  completed and inserts the receipt SHA-256 in one byte-preserving atomic
  replacement. A stale pending snapshot fails; an already completed identical
  binding succeeds without a write; a different binding is a conflict.
- A fulfillment request must name the checkpoint's exact canonical
  repository-relative receipt path and the exact normalized operator identity
  already recorded inside the receipt. Receipt bytes are loaded and hashed
  before publication and re-read immediately before the task mutation.
- The existing valid shared graph is a prerequisite to mutation. Fulfillment
  does not directly promote dependents: after publication, the repository
  adapter reloads and verifies the bound receipt, and only the shared scheduler
  can classify the checkpoint completed and select newly unlocked work.
- `taskmodel.Task` carries the checkpoint state and receipt identity used by
  read surfaces. App projection assigns `awaiting`, `fulfilled`, or `invalid`;
  CLI and TUI render those values and never reconstruct checkpoint policy.
- Operator checkpoints remain pre-authored, never-agent-executed external
  evidence. Autonomous `needs_input` remains a runtime supervisor question
  with durable question revisions, answers, and resume history.

## OC-01 Operator Checkpoint Receipt Authority (2026-07-14)

- `operator-checkpoint-v1` is a canonical dependency identity but never an
  executable workflow. Its only canonical statuses are pending and completed;
  the repository adapter projects pending as `awaiting_operator`, and neither
  pending nor completed checkpoints can enter a Codex-ready selection.
- Every checkpoint declares exactly
  `.agent/checkpoints/<task-id>/receipt.json`. Pending checkpoint metadata is
  deliberately unbound; completed metadata binds the exact receipt bytes with
  `checkpoint_receipt_sha256`. OC-02 must publish that binding and status
  transition atomically rather than using the generic status updater.
- `operator-checkpoint-receipt-v1` is a closed, dependency-free JSON contract.
  It contains accepted outcome, operator/provenance, explicit UTC time,
  subject/decision, typed repository-relative evidence paths with lowercase
  SHA-256 identities, and optional build/source hashes. Evidence bytes,
  secrets, private payloads, arbitrary maps, unknown fields, and duplicate
  fields are outside the contract.
- Canonical task parsing owns exact receipt-path and workflow-metadata rules.
  Read-only receipt loading owns non-symlink regular-file safety, strict
  content validation, task matching, and exact byte identity. The repository
  scheduler adapter owns reconciliation of the bound hash against current
  receipt bytes.
- `taskscheduler` remains the final pure admission boundary: awaiting authority
  classifies as operator input, while completed authority must carry a valid
  bound hash and a verified adapter result. Missing, malformed, mismatched, or
  identity-drifted completed receipts invalidate the graph and clear all
  selections; only verified completion satisfies dependency edges.
- General app projection may carry checkpoints as non-worker tasks so shared
  graph evidence remains readable. Command mutation, replay semantics, and
  checkpoint-specific CLI/TUI rendering are intentionally deferred to OC-02.

## TS-04 Autonomous Shared Scheduling Authority (2026-07-14)

- `internal/taskscheduler` is the only owner of autonomous graph validation,
  cycle detection, dependency semantics, ready ordering, selected-next choice,
  and occupied-task conflict classification. `internal/autonomousscheduler`
  remains only as a compatibility adapter carrying canonical task/state
  identity into autonomous task-run, queue, and child-publication boundaries.
- `internal/taskschedule` owns shared canonical task and lifecycle loading.
  `LoadActiveStrict` is the autonomous mutation/admission boundary requiring
  every autonomous state and child-publication authority; ordinary repository
  read projections use `LoadActive` so sparse state remains visible and policy
  is still evaluated once by the shared scheduler.
- Queue-local exclusion and fairness remain queue policy, but queue readiness
  and parallel occupancy conflicts consume shared `TaskReadiness` projections.
  Queues filter non-autonomous ready tasks without reordering or reclassifying
  autonomous candidates, and exact selected identity is retained on adapter
  nodes.
- Active autonomous task views load the same repository schedule as list/status
  with an autonomous selection scope. They render shared dependency issues and
  invalid diagnostics and explicitly distinguish the selected ready task from
  another ready task; they do not reconstruct lifecycle or archive policy.
- Only exact verified/reconciled completed archive authority satisfies an
  edge. Verified cancelled, abandoned, and superseded archives produce typed
  terminal-unsatisfied issues containing the immutable archive ID and terminal
  reason. Malformed or unverified archive authority invalidates the graph;
  supersession never redirects a dependency.
- Autonomous execution treats any nonempty shared invalid-graph diagnostic set
  as a pre-admission error. No task operation or worker may start from an
  invalid graph, and explicit task execution must classify that exact task as
  shared `ready` before admission.

## TS-03 Shared Read Projection Authority (2026-07-13)

- `internal/taskschedule` is the single repository-to-scheduler adapter shared
  by mixed execution and general read projections. It loads locally valid
  canonical task bytes without hiding duplicate identities, applies available
  autonomous lifecycle, verifies archive authority, and delegates every graph
  and readiness decision to `internal/taskscheduler`.
- One scheduler evaluation owns deterministic per-workflow selections for
  mixed-pass and autonomous tasks. The explicitly scoped `SelectedNext` remains
  execution authority; `WorkflowSelections` lets app views expose both ready
  identities without performing a second evaluation or recomputing policy.
- Canonical task projections remain in source-path order for stable human
  listing. Selection flags are exact task-ID/source-path matches against the
  shared result and are independent of display order. Typed readiness, exact
  unmet IDs, dependency issues, conflicts, and diagnostics travel with the
  projection; `StatusResult.Schedule` preserves the complete result.
- CLI and TUI are renderers only. A missing selected identity is rendered as
  `none`/`nothing runnable` even when pending tasks exist. TUI chooses the
  shared mixed marker before the autonomous marker because its general `R Run
  Once` control invokes mixed-pass execution; neither path scans for a pending
  substitute.
- Graph invalidity remains readable rather than becoming an arbitrary list
  error: every projected task has `invalid_graph`, the complete ordered
  diagnostics remain available, and all workflow selections are empty.
  Locally malformed canonical files still fail during taskfile loading before
  graph evaluation.
- Dependency-blind taskfile runnable/select APIs are removed, not deprecated.
  `taskfile` continues to own parsing, canonical source discovery, duplicate-
  fail-fast `List` compatibility for nonscheduler callers, and atomic metadata
  writes; scheduling consumers use the duplicate-preserving `LoadAll` adapter
  boundary instead.
- `internal/autonomousscheduler` remains temporarily only for autonomous
  execution, queue, and specialized readiness consumers. Removing that final
  duplicated graph policy and adapting occupancy/admission behavior is the
  bounded TS-04 task, not part of TS-03 read-surface migration.

## TS-02 Mixed Execution Scheduling Authority (2026-07-13)

- Mixed execution consumes one exact `taskscheduler.Result` and uses only its
  `SelectedNext`; it never scans pending tasks or reorders the ready set.
  `SelectionWorkflow` is explicit result authority so a single validated
  cross-workflow graph can select the correct executable workflow without
  hiding other readiness evidence.
- `taskfile.LoadAll` is the duplicate-preserving canonical load boundary for a
  graph consumer. It performs file/path/frontmatter validation and stable
  source discovery but no cross-file identity decision. Legacy `List` retains
  duplicate fail-fast behavior temporarily for unmigrated TS-03 read callers.
- Autonomous active dependencies use their validated durable lifecycle when
  state exists and otherwise retain canonical task status compatibility.
  Archive dependencies are admitted only after full archive verification with
  the live ledger, Git authority, and configured-secret absence checks; the
  shared scheduler alone assigns dependency meaning to admitted evidence.
- `runonce.Result.Schedule` is the execution projection. Graph diagnostics are
  returned as `ScheduleError`, while a valid mixed selection that has no ready
  task and at least one terminal-unsatisfied mixed dependent returns
  `TerminalDependencyError`. Both stop before ledger run creation and Codex;
  temporary waiting/block/input and an empty mixed queue remain no-task.
- Terminal-unsatisfied tasks are reported both globally and in a scheduler-
  owned selection-workflow subset. Execution consumes the subset, preventing
  an unrelated autonomous terminal edge from turning an empty mixed run into
  an error.
- Bounded loops do not pin or cache a mixed task. Every production pass calls
  `runonce.Run`, which reloads and re-evaluates canonical graph authority after
  the prior pass's atomic metadata update. This is what unlocks dependents and
  prevents surplus selection from stale state.

## TS-01 Pure Shared Scheduler Contract (2026-07-13)

- `internal/taskscheduler.Evaluate` is the pure shared boundary. Canonical task,
  workflow-state, archive, and occupancy owners supply normalized value
  evidence; the scheduler performs no I/O and returns a complete map-free
  `revolvr-task-schedule-v1` value suitable for deterministic JSON.
- Effective scheduling state is explicit rather than inferred inside the
  graph. This lets mixed-pass canonical status, autonomous durable lifecycle,
  and future checkpoint fulfillment remain owned and validated by their
  respective adapters while one package owns their dependency meaning.
- Graph invalidity is data, not a lossy first error. Typed diagnostics are
  sorted deterministically; no task is ready or selected when any diagnostic
  exists, and every task receives `invalid_graph` readiness so read surfaces
  can explain an execution refusal.
- An unsatisfied edge records both its exact dependency ID and typed per-edge
  evidence. Overall task precedence is terminal-unsatisfied over needs-input
  over blocked over ordinary waiting. This prevents a permanent terminal edge
  from being hidden by a temporary blocker while retaining all contributing
  edges for rendering.
- Operator checkpoints in pending/awaiting state are explicit operator-input
  nodes and never enter the ready set. Their dependents wait with an exact
  `awaiting_operator_dependency` edge reason. OC-01 remains responsible for
  validating receipt authority before it may adapt a checkpoint as completed.
- All non-ready task/category projections are canonical source-path/task-ID
  ordered. Priority is applied only after readiness is proved, using
  prioritized before unprioritized, lower numeric value, slash-normalized
  source path, and task ID. `SelectedNext` is an exact copy of the first ready
  projection eligible for the explicit selection workflow, or the first ready
  projection overall when selection scope is empty.
- Archive inputs are admitted only with a nonempty archive identity and exact
  verified/reconciled authority. Completed satisfies an edge; cancelled,
  abandoned, and superseded remain terminal-unsatisfied and require an
  explicit reason. Active/archive ambiguity and duplicate archive task
  identity invalidate the graph.

## Scheduling, Checkpoint, and Migration Campaign Boundary (2026-07-13)

- `internal/taskscheduler` will become the sole workflow-aware authority for
  graph construction/validation, dependency readiness, deterministic ready
  ordering, typed reasons/diagnostics, selected-next decisions, and applicable
  conflicts. `internal/taskfile` retains canonical parsing, validation,
  loading, and atomic metadata writes but no independent selection policy.
- The shared result deterministically separates ready, exact dependency-
  waiting, dependency-blocked, terminal-unsatisfied, operator-input/checkpoint,
  conflict-blocked, and invalid-graph tasks and identifies at most one selected
  next task. Invalid graphs fail execution closed while read surfaces retain
  typed diagnostics.
- Only completed dependencies satisfy edges. Pending/running wait; blocked and
  needs-input retain distinct dependency reasons; cancelled, abandoned, and
  superseded are terminal-unsatisfied and supersession never redirects an
  edge. Only verified/reconciled completed archives satisfy dependencies; all
  other archive dispositions are explicit terminal-unsatisfied evidence.
- Missing or duplicate task identities, duplicate/self edges, active/archive
  ambiguity, malformed archive authority, and cycles invalidate the graph.
  Ready ordering is prioritized before unprioritized, then lower numeric
  priority, canonical slash-normalized source path, and task ID. Priority does
  not make an unready task runnable.
- Mixed execution, bounded loops, app projections, CLI/TUI reads, and
  autonomous selection/readiness must consume the same result. Rendering may
  not recompute policy, and every first-pending fallback and apparently
  authoritative dependency-blind selector must be removed after migration.
- Canonical frontmatter is closed by default. Recognized and `x-` extension
  keys are the only admitted keys; both reject duplicates. Extensions remain
  byte-preserved and inert. Unsupported-key diagnostics bind canonical source
  path, one-based frontmatter line, and exact key without introducing a broad
  YAML dependency.
- `operator-checkpoint-v1` is a separate never-agent-executed canonical
  workflow and dependency identity. It awaits an explicit operator fulfillment
  against a strict, versioned, dependency-free JSON receipt containing only
  safe evidence references and SHA-256 identities. Completed checkpoint
  receipt absence, mismatch, malformation, or identity drift invalidates the
  graph. Autonomous `needs_input` remains a separate runtime supervisor
  question with durable answer/resume history.
- Autonomous bulk migration initially admits only pending mixed-pass implement
  tasks with no autonomous state, lineage, or unrepresentable prior execution.
  Planning is deterministic and batch-wide before mutation. Application is
  locked, journaled/restartable, state-before-task, exact-replay idempotent, and
  conflict-fail-closed; it never fabricates evidence, overwrites user evidence,
  converts silently during init, invokes Codex, or commits.
- `/home/gernsback/source/cyber-arpg` remains read-only throughout the campaign.
  Only QA-01 may perform its specified final load/graph/projection and migration
  dry-run assessment, without running Codex or applying migration. Dependency-
  heavy tasks remain mixed-pass unless another reason independently justifies
  migration.

## R2-11 Artifact-GC CLI Error Composition (2026-07-13)

- Artifact-GC operation and result rendering are independent fallible effects.
  Once both have been attempted, the CLI returns `errors.Join(operationErr,
  writeErr)` so callers can inspect either cause and neither failure masks the
  other.
- Resume renders only when recovery returned a journal operation identity,
  because that identity is the minimum resumable evidence. Apply passes every
  returned result to the renderer; an empty journal remains intentionally
  silent.
- CLI retention-operation callbacks are nil-compatible test seams. Production
  commands always default them to the app-owned plan, apply, and resume
  functions; rendering and error composition remain CLI-owned.

## R2-10 Bare Run Contract (2026-07-13)

- `revolvr run` with no mode flag means one selected harness pass. It is
  behaviorally identical to `revolvr run --once`; the flag remains a supported
  explicit spelling rather than a distinct mode.
- Explicit bounded-loop, autonomous task, queue, and daemon modes continue to
  take precedence only when their corresponding mode flag is selected. Their
  mutual-exclusion, option ownership, and positive-bound validation are
  unchanged.
- A bare run uses ordinary one-pass result and error semantics. No task is a
  successful informative result, committed work uses the normal summary, and
  runner/config failures are nonzero. A placeholder may never turn a missing
  operation into successful automation.

## R2-09 Verification Command Presence (2026-07-13)

- `verification.commands` has three semantic states. Omitted and YAML `null`
  mean no sequence override and inherit the supplied base. A present sequence
  is authoritative: nonempty replaces inherited commands, while `[]` replaces
  them with a nonnil empty set.
- Any authoritative flat-command sequence clears an inherited tiered plan.
  Conversely, an explicit tiered plan continues to clear flat commands. A file
  that specifies both remains invalid even when one sequence is empty.
- `runonce` synthesizes the repository-sensitive default only when both flat
  commands and the tier plan are nil. It must not collapse a nonnil empty slice
  to nil. Missing-command verification, preflight, and commit policies then
  decide whether the intentional empty set is acceptable.
- Effective configuration fingerprints describe resulting behavior rather than
  YAML spelling. Thus omitted and `null` hash identically when they inherit the
  same base; explicit empty differs from an inherited Go default.

## R2-08 Ledger-Export Record Size Contract (2026-07-13)

- One ledger-export record may contain at most 16 MiB of canonical JSON bytes;
  the JSONL newline delimiter is not part of that limit. Exact-limit records
  are valid, while limit-plus-one records are not.
- The writer owns admission. It must marshal and size-check every run and event
  record before mutating the output stream, so an oversized ledger snapshot
  fails before any immutable export artifact or runtime directory is created.
- Verification, replay validation, and snapshot replay consume the same parser
  and therefore the same limit. Explicit line extraction replaces Scanner's
  separate maximum-token behavior and keeps writer/reader boundary semantics
  byte-identical.
- Ledger task text and event payloads remain valid live-ledger data above this
  bound; they are not silently truncated or rewritten. Such a snapshot is
  explicitly ineligible for this versioned export format until the source data
  or a future format changes the contract.

## R2-07 Retention Rename Durability (2026-07-13)

- A prune representation is not eligible for a completed-action journal until
  its cross-directory rename is durable on both sides. GC syncs the source
  directory, then the quarantine destination from the representation's leaf
  directory upward through the operation directory. The upward chain makes
  every directory entry created by `MkdirAll` durable as well as the moved
  representation.
- Filesystem-ahead recovery repeats those syncs even when no rename remains to
  perform. Quarantine presence proves an effect may be reconciled; it does not
  prove the prior process made either directory side durable.
- Quarantine cleanup is durable before the terminal `cleaned` claim:
  `RemoveAll` is followed by an operation-directory sync. The preceding
  `completed` journal remains the recovery boundary if removal or that sync is
  interrupted.
- Apply failure injection is an explicit deterministic test boundary after
  each rename, directory sync, journal publication, and cleanup mutation. It
  does not alter production behavior when unset.

## R2-06 Queue History Authority (2026-07-13)

- A queue operation begins with one pristine `admitted` record at sequence
  zero. Canonically named immutable history records must then form one complete
  sequence with nondecreasing timestamps. Foreign names, gaps, duplicate
  sequences, malformed records, and isolated high-sequence records fail closed.
- Immutable operation material is the operation ID, mode, configuration schema
  and hash, safety identity, task bound, effective worker bound, start time,
  sweep, and daemon-wake authority. It cannot change within a queue. The only
  schema change is the existing one-way v1-to-v2 migration at the same stage;
  its in-flight selection and accumulated evidence must be preserved exactly.
- Legal v2 execution transitions are admission to selection or terminal;
  selection to one task stop; and task stop to another task stop, a new
  selection, or terminal. Repeated task-stop records are required for parallel
  reconciliation and each must append exactly one outcome, advance statistics,
  and convert exactly one admitted slot to matching terminal evidence.
- `operation.json` is a mutable cache, never recovery authority. It may be
  missing or be an exact older history entry. It may not exist without history,
  lead history, or conflict at its claimed sequence. Persistence revalidates
  the complete history and its current predecessor before appending exactly one
  transition.

## R2-05 Queue and Child Runtime-Path Boundary (2026-07-13)

- Autonomous queue and child-publication state is harness-owned runtime data
  under the same `internal/runtimepath` trust boundary as task-run and outer
  execution state. Canonical-root resolution happens once at entry. Every
  existing or newly created ancestor must be a non-symlink directory without
  group/world write permission; every protected final file must be regular,
  non-symlink, non-group/world-writable, and single-linked.
- Protected reads no longer check a path and then call `os.ReadFile` or
  `os.ReadDir`. `runtimepath.ReadFile` and `ReadDir` use no-follow opened
  descriptors, prove named/opened identity before consuming bytes or entries,
  and prove it again afterward. `CheckOpenedDir` and `SyncDir` extend the same
  identity rule through directory enumeration and durability boundaries.
- Queue persistence uses `EnsureDir`, protected reads, exclusive opened-file
  checks, temp identity checks, pre/post-rename checks, protected directory
  sync, and safe temp cleanup. Its operation lock is checked before open,
  immediately after open, and after successful flock. The former local
  symlink-only ancestor walker and root resolver are removed. R2-06 remains
  responsible for semantic transition-chain authority.
- Child publication applies the same sequence to its publication lock,
  immutable history and initial-state link publication, mutable checkpoint
  rename, state readback, and directory sync. Immutable link targets and
  mutable rename targets are rechecked immediately before mutation; temporary
  cleanup removes only a still-protected named file and will not follow a
  replaced parent into an outside tree.
- Deterministic fault hooks exist at opened-lock, immutable-link, and mutable-
  rename boundaries solely to verify the production rechecks. Outside-sentinel
  tests require unchanged content, mode, hard-link count, and directory names
  for every rejected ancestor/final-file or substitution scenario.

## R2-04 Shared Child-Publication Authority (2026-07-13)

- Child publication has one shared read-only authority owner,
  `internal/autonomouschildpublication`, so publisher recovery and scheduler
  admission cannot interpret the same evidence differently. The mutation
  coordinator remains `internal/autonomouschild`; the shared package publishes
  nothing and therefore does not invert the existing child-to-scheduler graph
  dependency.
- A valid journal has exactly one nonempty child set, strictly ordered by
  proposal key. Each task ID is rederived from parent, decision, proposal, and
  key; task/state paths are canonical; every artifact identity is lowercase
  SHA-256; creation time is UTC; and the only stage/sequence pairs are
  admission/1, states-published/2, tasks-published/3, and completed/4.
- Complete immutable snapshots remain the history format. Authority must be
  identical across a contiguous sequence-one-through-latest chain. A missing
  mutable checkpoint is recoverable and a stale checkpoint is valid only when
  exactly backed by its history entry; ahead, conflicting, unbacked, malformed,
  or gapped evidence is refused.
- Publisher replay recomputes the full expected journal authority from the
  validated decision/reference, parent task/state identities, explicit time,
  and deterministic child task/state bytes. The material hash is not trusted
  alone. Every authority field and exact child record must match before effects
  may be skipped, and claimed task publication is read back before completion.
- Scheduler admission consumes the same reconstructed projection. It requires
  completed history, exact child membership and canonical paths, matching task
  metadata and immutable execution-state lineage, and a child-record state
  hash that rederives from that lineage. Current task/state bytes may evolve
  through their separate validated lifecycle; `ChildOf` cannot change or
  disappear under a legal execution-state transition.
- Runtime-path identity, mode, hard-link, ancestor, and open/rename protections
  remain R2-05. R2-04 changes semantic recovery authority without duplicating
  that upcoming filesystem boundary.

## R2-03 Immutable GC Recovery Authority (2026-07-13)

- The authoritative GC journal is the complete, contiguous series of
  canonical immutable snapshots named by sequence, beginning with admission at
  sequence one. The mutable `journal.json` is only a cache: it may be absent or
  point to an older byte-equivalent history entry, but it may not be ahead,
  conflict with its backing entry, or exist without history. Gaps, directories,
  and foreign history names fail closed. A regular atomic-writer temporary is
  explicitly uncommitted and ignored so a crash before rename can retry.
- Every history entry retains one exact validated plan and operation. Legal
  transitions require the next sequence, a nondecreasing UTC timestamp, the
  precise stage/cancellation combination, an immutable valid export ID, and an
  exact prefix of ordered mutating action paths. History is published before
  the checkpoint cache. Older chains that encode an invalid or ambiguous state
  are refused rather than normalized into destructive authority.
- Effects precede the immutable history claim. Recovery therefore verifies
  every claimed compression against canonical manifest and exact gzip/source
  identities, and every claimed prune against absent source representations
  plus its exact quarantined representation. An unclaimed filesystem-ahead
  effect remains eligible for the existing idempotent action reconciliation.
- Quarantine removal occurs after the durable `completed` state, so recovery
  accepts a fully removed quarantine while that is the latest state. Partial or
  conflicting quarantine evidence is rejected. `cleaned` is terminal only
  when the operation quarantine is absent and every claimed action remains
  reconciled.
- A nonempty export ID is never sufficient prune authority. Every recorded
  export is verified and replay-validated on apply/resume and inspection, then
  its canonical manifest must match the plan's operation, frozen time, policy,
  complete bounds, high-water mark, no-predecessor contract, and WAL-safe
  logical ledger identity. This check also gates terminal replay.

## R2-02 Artifact-Retention Exclusion Contract (2026-07-13)

- This decision supersedes AW-25's check-and-release probe rule. Mutating GC
  owns four OS advisory locks for its complete transaction, ordered artifact
  retention, autonomous execution, Git administration, then child
  publication. Every file is created if absent. Only the outer retention lock
  waits; inner locks are nonwaiting admission checks, and partial acquisition
  releases in reverse order.
- Archive/reopen and queue/direct autonomous coordination remain excluded by
  the held autonomous-execution lock. Workspace Git administration is excluded
  by the held Git-admin lock. Child publication is excluded by its held lock.
  Those entrants do not also acquire retention, avoiding redundant nesting.
- Every successful source-writer acquisition first takes a shared
  control-root artifact-retention lock and holds its open file description
  through heartbeat-protected work until release. Workspace writers use the
  control root rather than their execution worktree, and distinct workspace
  writers may coexist under shared locks. GC takes the same inode exclusively.
- The apparent autonomous/retention inversion is bounded deliberately: GC
  never waits for an autonomous lock after taking retention. If an autonomous
  owner is already active and waits for shared source admission, GC's inner
  probe fails and releases retention, allowing the owner to proceed. GC waits
  only when a source writer already owns a shared gate and holds no inner lock
  while waiting.
- Source release always relinquishes its shared gate, including cancellation,
  expiry, ownership conflict, and persistence-error paths. Because metadata
  cleanup can fail or an older process may not participate in the gate, GC
  scans both control-root and every workspace source-writer namespace after
  exclusive acquisition and refuses any unexpired owner.
- This is a cooperative cross-process exclusion contract; R2-02 does not
  replace the established lock/path security boundary. Queue and child path
  hardening remains R2-05.

## R2-01 WAL-Safe Logical Ledger Authority (2026-07-13)

- SQLite file bytes, WAL sidecars, and checkpoint state are not logical ledger
  authority. The shared authority is schema
  `revolvr-ledger-logical-snapshot-v1`: a domain-separated, length-prefixed
  hash stream over the snapshot high-water mark, ordered run and event counts,
  every run field, and every event ID/run/type/time with exact payload bytes.
  Optional pointers/payloads have explicit presence markers, integer widths
  are fixed, and times normalize to UTC RFC3339 nanoseconds.
- `byte_size` beside this identity is the canonical logical stream size, not
  the SQLite main-file size. Source path admission, regular-file/mode/link
  validation, and before/after inode identity remain separate protections and
  must not be inferred from the logical digest.
- New ledger exports carry the logical identity schema in `source_ledger`, and
  that tag is part of the content-derived export ID. Verification recomputes
  the authority ID before trusting the tag and compares current logical state
  when the live high-water equals the export high-water. A later high-water
  remains coverage rather than exact-current-state authority.
- Existing untagged v1 export manifests remain structurally compatible. Their
  physical hash is treated as diagnostic only; immutable records, counts,
  bounds, hashes, replay, and live high-water coverage remain verified without
  presenting the raw SQLite hash as logical proof. Removing a logical tag from
  a new manifest invalidates its export ID.
- GC planning uses two transactional logical snapshots around inventory and
  requires equality plus stable source-file identity. Every action
  revalidation compares the tagged logical identity and high-water before
  mutation. Existing untagged plans are refused and must be replanned because
  physical hashes cannot safely authorize a new mutation.

## AUD-16 Proven Dead-Code Boundary (2026-07-13)

- Cleanup is limited to values whose lack of effects was established from the
  complete expression and its call sites. The safety no-op block, blank import
  anchors, queue's unread private parameter, metrics' unused map key,
  finalization's post-use task assignment, and audit's redundant root
  assignment have no interface, validation, persistence, or documentation
  role and are removed directly.
- A discarded return value does not make its producing call dead. Notification
  `Inspect` remains responsible for durable intent/payload/journal validation;
  planning apply remains responsible for state persistence; and audit
  resolution continues using its prepared root for canonical artifact
  readback. Only unused returned components or redundant assignments are gone,
  while calls and error authority remain explicit.
- AUD-16 introduces no helper, public API, dependency, broad rename, or TUI
  refactor. The admitted cleanup lowers production `internal` Go code from
  59,976 to 59,956 physical lines, 20 fewer, with focused, full, vet, and race
  verification.

## AUD-15 Canonical-State Replacement Boundary (2026-07-13)

- The only new persistence abstraction is package-local
  `Store.replaceState`. It applies exclusively to the eight autonomous-state
  owners whose canonical `state.json` replacement order is identical:
  same-directory temporary creation and mode, exact write, file sync and close,
  locked expected-state recheck, atomic rename, directory sync, and canonical
  readback.
- The helper owns the generic state fault-point vocabulary at those exact
  boundaries. In particular, optional-role and finalization commits now expose
  the generic boundaries they previously omitted; this standardizes injected
  crash coverage without changing the production write order.
- Owners retain lifecycle and request validation, immutable-history authority
  and ordering, operation replay/conflict rules, state identities, transition-
  specific readback predicates, and returned dispositions. The helper is not a
  generic store or state-machine framework.
- Autonomous queue and task-run persistence remain independent. Their
  checkpoint/history recovery and failure ordering are not equivalent, and
  task-run's AUD-11 ancestor/final-path revalidation and protected cleanup must
  not be weakened to fit the canonical-state helper.
- The refactor is admitted by the conditional LOC gate: production
  `internal/autonomousstate` Go code falls from 4,346 to 3,937 physical lines,
  409 fewer, with no dependency and with all eight call sites clearer.

## AUD-14 Autonomous-State Flock Wait Contract (2026-07-13)

- Autonomous-state serialization remains an OS advisory exclusive `flock` on
  the existing per-task `state.lock`; no in-process mutex or notification
  authority is introduced. All planning, audit, attempt, block, finalization,
  input, optional-role, and workspace mutations continue through the one
  shared acquisition helper.
- Acquisition checks caller cancellation first, then attempts
  `LOCK_EX|LOCK_NB` immediately. Success returns without constructing a timer.
  Errors other than `EWOULDBLOCK`/`EAGAIN` retain their original identity and
  return immediately.
- Contention uses a deterministic exponential delay of 1, 2, 4, 8, and 16
  milliseconds capped at 20 milliseconds for every later attempt. This bounds
  syscall rate and release-detection latency without claiming strict fairness
  or building a process-local waiter queue around a cross-process lock.
- Each delay owns one timer. Receipt of the timer fully consumes it; caller
  cancellation stops it and nonblockingly drains an already fired value before
  returning `ctx.Err()`. Cancellation is checked again before the next syscall,
  so it is prompt both before acquisition and throughout contention.
- Tests use separate file descriptions of the same real lock file rather than
  mocked mutexes. Process CPU-versus-wall-time coverage guards against
  reintroducing a spin, many-waiter coverage proves eventual acquisition in the
  admitted contention pattern, and benchmarks keep the uncontended zero-allocation
  path and contended wait cost visible.

## AUD-13 Constant-Query Ledger Snapshot Contract (2026-07-13)

- A multi-run ledger read uses one read transaction and at most two SELECT
  statements: one establishes the exact ordered run set and one returns events
  belonging to that set. A task/limit selection is repeated as a derived table
  in the event statement, so the SQL shape remains constant beyond SQLite
  variable limits and cannot diverge from the first selection inside the same
  transaction snapshot.
- The full snapshot event statement joins `events` to `runs`; task history
  joins the limited task-derived table and orders by that run order followed by
  event ID. The public run slice remains in the first query's authoritative
  order, and each run's events remain ascending by global event ID.
- A run-ID map may be used only to find the already ordered destination index.
  Output is never assembled by iterating that map. Event rows append directly
  to the returned slices, which avoids a duplicate all-events buffer; the only
  additional size-dependent structure is one integer index entry per returned
  run.
- Evidence reads deliberately use `scanEvidenceEvent`, preserving malformed
  JSON bytes for downstream provenance decisions. Nil database payloads, runs
  without events, and `MaxEventID` retain their prior exact semantics.
- Any query, scan, close, context, or transaction-commit failure returns no
  partial public snapshot. Live read-only retry remains outside the whole
  two-query operation, so a busy retry starts a fresh transaction rather than
  mixing versions. Successful queries continue to commit the read transaction
  before publishing results.
- `GetRunWithEvents` remains a naturally constant two-statement lookup and was
  not merged into the multi-run helper. No schema, cache, invalidation policy,
  or public pagination API is introduced by AUD-13.

## AUD-12 Strict Configuration Decode Contract (2026-07-13)

- A config file is one YAML stream containing exactly one document. A document
  start/end marker is legal, as are trailing whitespace and comments, but any
  second document is an error even when empty. Stream structure is inspected
  first, then the admitted document is decoded independently with the existing
  strict known-field schema.
- Numeric omission is distinct from every explicit scalar. Pointer-backed
  decoded fields retain ordinary omission, while the YAML node pass rejects
  explicit null/empty numeric values before they can collapse to nil. Typed
  decoder failures are correlated with node line metadata and reported with
  exact dotted paths and sequence indexes.
- Configured execution timeouts and output caps are positive-only. So are
  notification attempts/caps, queue workers, and retention operation caps.
  Retention ages, recent-run/minimum-size values, and notification retry delay
  are deliberately nonnegative because zero already has a valid disabled/none
  policy meaning. Omission alone retains the caller's existing/default value.
- Seconds are decoded as `int64` and must not exceed the largest whole-second
  value whose multiplication by `time.Second` fits in `time.Duration`.
  Component-specific upper policy bounds, such as notification timeout and
  retry limits, remain separate and unchanged.
- Effective configuration fingerprints continue to derive only from normalized
  effective behavior. YAML markers and comments cannot affect them; valid
  pre-existing numeric values retain the same projection and hash. Only files
  that relied on a formerly ignored invalid value change from success to a
  field-specific failure.

## AUD-11 Harness Runtime Path Trust Contract (2026-07-13)

- The caller's repository root is resolved to one canonical directory before
  runtime-path work begins. That root is the trust boundary. Every existing
  component below it is examined with `Lstat`; harness-owned directories may
  not be symlinks, non-directories, or group/world writable. Missing directory
  components are created and revalidated one at a time rather than delegated
  to a path-following recursive create.
- A protected runtime file may be absent only at an explicit creation boundary.
  If present, it must be a regular non-symlink file, not group/world writable,
  and have exactly one hard link. After a sensitive open, both descriptor and
  named-path metadata must satisfy that contract and `os.SameFile` must prove
  they identify the same inode before any write or `flock` authority is used.
- Task-run immutable history, the mutable task-run checkpoint, task-run
  `operation.lock`, and the outer `autonomous-execution.lock` all use this
  policy. Reads are protected as well as mutations so replay/recovery cannot
  accept evidence through an unsafe path. The queue store retains its existing
  independent hardened implementation; AUD-11 does not begin AUD-15 storage
  lifecycle deduplication.
- Checkpoint persistence validates the history final before exclusive create,
  validates the opened history descriptor before writing, and validates the
  completed history file afterward. It validates the existing checkpoint and
  temp file immediately before rename, then validates the checkpoint and
  operation directory after rename before directory sync.
- Cleanup is also a path-sensitive mutation. A checkpoint temp is unlinked only
  when its current repository-relative path still resolves to a valid protected
  regular file. If an ancestor was substituted, cleanup deliberately leaves
  inspectable harness-owned residue rather than risking an outside-root unlink.
- Diagnostics identify the first unsafe repository-relative component and do
  not resolve or disclose a symlink target. Deterministic tests must preserve
  an outside sentinel's directory entries, contents, permissions, and hard-link
  count across every rejected ancestor, final-file, and rename-boundary
  substitution.

## AUD-10 Notification Cancellation Persistence Contract (2026-07-13)

- A transition returns journal authority even when it returns an error. Before
  a new immutable history entry is valid, that authority is the caller's prior
  journal. After the complete next history entry is visible, that entry is the
  newer authority even if checkpoint replacement or a later sync reports an
  error. A known-good journal may never be replaced by a zero value merely to
  report persistence failure.
- Immutable transition history remains ordered before the mutable
  `journal.json` checkpoint. Write, file-sync, or close failure before a new
  history file is complete removes that file and syncs the removal, leaving the
  prior transition authoritative. Once complete history exists, reconciliation
  derives the journal from history; a checkpoint that is absent or behind is a
  recoverable cache, not authority to discard the transition.
- Every delivery cancellation path attempts a bounded resumable transition
  while still holding the delivery lock. Transition errors are joined with the
  original cancellation so both remain inspectable. With no persistence error,
  the original cancellation value is returned unchanged. Hook cancellation
  preserves caller deadline cancellation when present and otherwise retains
  the established `context.Canceled` behavior.
- Persistence failures are reconciled synchronously under the same delivery
  lock. The returned journal must equal either the exact prior journal or the
  exact next journal reconstructed from immutable history; any other observed
  sequence, identity, or stage is an additional reconciliation error rather
  than inferred state.
- Restart tests inspect immutable history and the raw mutable checkpoint
  separately before invoking delivery again. A restart may therefore observe
  the prior state or the complete transition according to the failed boundary,
  but it may not invent a transition absent from history or lose an attempt
  already present in history.

## AUD-09 Literal Git Staging Contract (2026-07-13)

- Repository paths supplied by machine evidence are opaque exact filenames,
  not user-authored Git pathspecs. Every production staging command that
  carries such a list uses the global `--literal-pathspecs` option before
  `add`, followed by `--` before the paths. The contract covers the primary
  commit gate, autonomous archive commits, and restored-task recovery staging.
- `--` remains necessary to end option parsing but is not treated as protection
  from colon magic or wildcard expansion. Conversely, pathless whole-tree
  staging such as `git add -A` needs no literal-pathspec mode because there is
  no generated path argument to interpret.
- Exact path normalization may remove only the impossible empty Git filename
  and exact duplicates, then sort bytewise for deterministic evidence. It may
  not trim, normalize, reject, or encode legal whitespace, control-byte,
  leading-dash, colon-leading, wildcard-looking, or non-UTF-8 names merely to
  simplify staging.
- Deletions are staged by passing the deleted path. Rename evidence carries
  both the source and destination, and literal add stages both sides. Recovery
  code that inspects recorded commands must account for the global option when
  identifying the `add` subcommand.
- Real-Git tests are authoritative for this boundary. They inspect the index
  and cached changed-path set after the production add but before commit, then
  compare the committed tree to that exact expected set. Matching files that
  were not in the captured machine list must remain outside the index.

## AUD-08 Complete Git-Status Evidence Contract (2026-07-13)

- Every authoritative dirty/changed capture uses porcelain v1 with `-z` and
  `--untracked-files=all`. Human short-status output is not parsed. Each record
  is exactly two status bytes, one separator, and an arbitrary non-NUL path;
  rename/copy records contain the destination followed by a separate NUL-
  delimited source path, matching Git's reversed `-z` field order.
- The parser accepts Git's documented v1 status alphabet, preserves filename
  bytes in Go strings without UTF-8 normalization or whitespace trimming,
  rejects malformed, empty, unterminated, or incomplete rename/copy fields,
  and derives a unique byte-sorted set containing both sides of a rename/copy.
- A status capture is authoritative only when the runner completed without
  error, timeout, nonzero exit, stdout truncation, or stderr truncation and the
  entire NUL stream parsed successfully. Otherwise `CaptureError` is required,
  the bounded stdout/stderr previews and truncation counts are diagnostic only,
  and no entry/path/dirty/changed collection may be populated.
- Git output caps remain real integrity bounds, not values to bypass. A status
  larger than the configured cap fails clearly and makes no mutation; callers
  may configure a larger finite cap to obtain a complete set. Raising a cap
  never converts a truncated prefix into evidence.
- Preflight, direct runs, autonomous cycles/corrections, workspace operations,
  and commit already gate on `CaptureError`; commit additionally suppresses
  path lists from erroneous captures before any HEAD lookup or mutation.
  Autonomous archive now treats truncation as command failure too, including
  status admission and parsed HEAD/diff/show results. This closes alternate
  staging, checkpoint, and completion paths around the canonical capture.
- AUD-08 governs completeness and unambiguous status parsing. Literal Git
  pathspec semantics for the complete machine-generated path set remain the
  separate AUD-09 task.

## AUD-07 Atomic Last-Message Publication Contract (2026-07-13)

- The canonical last-message artifact is parent-owned. Codex receives only a
  deterministic same-directory raw staging path, precreated as `0600`; public
  artifact metadata continues to name the canonical path. `PrepareInvocation`
  includes the staging path in argv so precomputed provenance remains exactly
  equal to the command that actually runs.
- The recognized per-canonical recovery set is the canonical file plus hidden
  raw and redacted temporary siblings. Invocation preparation removes all
  three, syncs the directory when anything was removed, and creates a new
  empty raw staging file. Thus a restart cannot consume stale raw content or
  publish an abandoned redacted temporary.
- Publication accepts only a regular, non-symlink raw staging file with no
  group/world permissions. Nonempty bytes are redacted as one complete value
  before any canonical publication. The parent writes a new `0600` temporary,
  changes it to the established `0644` artifact mode only after safe content
  is present, syncs and closes it, and atomically renames it to canonical.
- After rename, the raw staging file is removed, the canonical regular-file
  type and exact mode are verified, and the parent directory is synced.
  Handled failures clean both temporary siblings and sync their removal. A
  directory-sync failure after rename may leave canonical output visible, but
  those bytes have already passed redaction; unredacted canonical output is
  prohibited at every boundary.
- A missing or zero-byte child result is missing output and publishes no
  canonical file. Returned parsing remains `TrimSpace`. With no configured
  redactor, exact child bytes are preserved in the canonical artifact; with a
  redactor, successful nonempty output retains the prior trimmed content plus
  final newline. The restricted raw temporary may contain live secrets, which
  is the explicit privileged-actor non-goal from AUD-07.

## AUD-06 Streaming Codex Metrics Contract (2026-07-13)

- The authoritative Codex JSONL artifact is parsed incrementally through
  `jsonl.ReadRecords`; production metrics extraction may not load or cap the
  total artifact. Reads use a fixed 32 KiB buffer and retain at most one record
  under the shared 1 MiB `jsonl.MaxRecordBytes` contract.
- `receipt.ParseCodexUsageMetricsReader` is the single metrics parser.
  Byte-slice and owned-file entry points delegate to it, as do reader/file
  receipt rewrite entry points. Codex execution, mixed-pass receipt parsing,
  and autonomous worker receipt parsing use the owned-file path.
- The owned-file path closes on every return. While parsing, cancellation also
  closes the descriptor to interrupt a blocked read; context cancellation is
  checked again before every delivered record so a large pipe chunk cannot
  postpone cancellation across multiple records.
- Malformed records are scanned through so diagnostics can report their count
  and partial observed totals, but any malformed record makes metrics
  incomplete. Callers may inspect those partial totals but must not publish or
  rewrite receipt metrics from them. `MalformedCodexJSONLError` names the first
  record/count without retaining record content.
- Numeric conversion uses `json.Number` and checked integer conversion.
  Individual values and aggregate additions that exceed native receipt integer
  range fail with `CodexUsageOverflowError`, naming the record and field.
  Silent float conversion or integer wraparound is prohibited.
- A record beyond 1 MiB retains the AUD-05 `jsonl.ErrRecordTooLarge` identity.
  File open/read/close failures are separately typed as
  `ErrCodexJSONLSource`. In Codex results only source failures are artifact
  failures; cancellation or invalid/incomplete optional metrics are parse
  diagnostics. Neither category modifies the authoritative artifact.
- Receipt rewrite is fail-preserving: if metrics are malformed, oversized,
  overflowed, canceled, or unreadable, it returns the original receipt bytes
  and parsed receipt with `changed=false` plus the diagnostic. It never writes
  partial totals or requires truncating the audit stream.

## AUD-05 Authoritative JSONL Record Contract (2026-07-13)

- Runner stdout has three independent consumers: the bounded in-memory result,
  an optional bounded line preview, and an optional authoritative byte writer.
  A preview limit or capture cap never transforms bytes delivered to the
  authoritative writer, and authoritative writer errors retain their identity
  through the command result.
- Human line previews are limited to 64 KiB including the literal
  ` [line truncated]` marker. An oversized logical line produces exactly one
  preview; bytes through its delimiter are discarded only from the preview,
  after which preview processing resumes at the next logical line.
- `internal/jsonl.MaxRecordBytes` is the shared hard machine-record limit:
  1 MiB excluding the newline delimiter. Complete records at that limit are
  accepted. An additional byte yields typed `ErrRecordTooLarge` and
  `RecordTooLargeError` evidence naming only the record number and limit; the
  incomplete record is never delivered to a persistence callback.
- JSONL records are reconstructed from raw bytes before conversion to text.
  Newlines delimit records regardless of pipe chunking, and a nonempty final
  record without a newline is emitted on close. This preserves split UTF-8 and
  gives AUD-06 one record-size contract to reuse for streaming metrics.
- Codex artifacts redact each complete record before writing and parsing it.
  Redaction therefore spans arbitrary read chunks, while malformed records are
  retained whole and reported through the existing parse-error evidence.
  Durable output uses a canonical trailing newline for every delivered record.
- A record-limit or artifact-writer failure is both an invocation error and an
  artifact error. Earlier complete records may remain as valid JSONL, but no
  prefix of the rejected record may masquerade as a record.

## AUD-04 Live Ledger Read Contract (2026-07-13)

- A raw SQLite ledger path is live evidence and must be opened through
  `OpenLiveReadOnly` with `mode=ro`, query-only enforcement, and ordinary
  SQLite locking/change detection. `immutable=1` is never valid for that API.
- Frozen immutable evidence is the verified ledger export and replay format.
  No raw SQLite immutable mode is retained because the repository has no raw
  database-copy boundary that proves database and sidecar stability for an
  entire connection lifetime.
- Any projection combining runs and events owns one read transaction. This
  includes full snapshots, per-task dossier history, and one-run history; a
  caller never combines rows from different committed database versions.
- Live-reader contention retains a five-second maximum but divides it into
  short driver busy intervals with context-aware retry. Cancellation returns
  joined context and SQLite busy evidence instead of remaining inside one
  uninterruptible busy handler.
- Read-only store mutation methods fail at the abstraction boundary with
  `ErrReadOnly`. They do not initialize schema, create directories or
  sidecars, invoke a writer clock, or attempt SQL mutation.
- In WAL mode, safe ordinary locking may update transient reader marks in an
  already-existing in-process `-shm` index. Those coordination marks are not
  durable ledger content. Live reads may not create, resize, replace, or chmod
  that sidecar and may not modify the database or WAL bytes; disabling locks to
  suppress shared-memory coordination is prohibited.

## AUD-03 Process-Tree Lifecycle Boundary (2026-07-13)

- Production commands execute inside a dedicated process group on supported
  Unix platforms. The group ID is the launched process ID captured at start;
  cancellation targets that group rather than a mutable ambient process
  relationship.
- Cancellation first sends `SIGTERM`, observes group liveness for the bounded
  command grace period, and sends `SIGKILL` to the same group if any member
  remains. Runner return joins the termination monitor, so descendants cannot
  retain worktree mutation authority after cancellation is reported.
- `exec.Cmd.WaitDelay` remains the pipe-draining bound. Cancellation and timeout
  classification, nonzero process exits, bounded output, and graceful output
  retain their existing caller-facing semantics.
- Non-Unix platforms without an implemented tree primitive fail closed with
  `ErrProcessTreeUnsupported` before starting a command. Direct-child killing
  is not represented as equivalent process-tree ownership.
- Process grouping is a lifecycle guarantee for ordinary descendants, not a
  sandbox or defense against a hostile child deliberately escaping its process
  group.

## AUD-02 Source-Writer Ownership Guard (2026-07-13)

- Source-writer ownership is an active operation invariant, not a best-effort
  background refresh. A shared guard monitors the lease, serializes heartbeat
  calls, retains the first ownership failure, and cancels the operation context
  so model and verification work stop promptly.
- Heartbeat failures and synchronous boundary-check failures are typed as
  ownership loss while preserving the underlying expiry, replacement-token, or
  persistence error. If cancellation, an operation failure, heartbeat failure,
  and release failure coincide, joined errors retain every applicable cause.
- Direct and autonomous execution synchronously recheck ownership before and
  after sensitive verification, source-transition, commit, and terminal
  publication boundaries. Autonomous ownership loss is unsafe for checkpoint
  or completion advancement and therefore returns a source-changed result.
- A lease whose recorded expiry has passed cannot be revived or released as
  current ownership. Replacement requires a fresh acquisition under the
  existing source-lock protocol.
- Monitor shutdown stops and joins the heartbeat loop before release. Healthy
  closure is silent, bounded, and releases exactly once.

## AUD-01 Ignored Source Evidence Policy (2026-07-13)

- Ignored repository state is denied by default because it cannot be replayed
  from the verified commit. Source capture inventories ignored entries with
  Git's NUL-delimited machine output, uses `Lstat` only for type
  classification, and reports relative quoted paths/types without file bytes
  or symlink targets.
- Harness-owned ignored state requires explicit caller authority and is limited
  to safe `.revolvr` directory/regular-file records in a control repository.
  Per-task execution worktrees never receive that allowance. `.git` internals
  remain Git-owned and outside ordinary source enumeration.
- `.revolvr` is not excluded by pathname from tracked or nonignored source.
  This distinction prevents user-controlled source from inheriting harness
  classification merely by choosing a runtime-looking name.
- Every autonomous source snapshot is an enforcement boundary. Admission,
  worker completion, verification completion, post-commit capture, workspace
  reopen/checkpoint, and final completion revalidation all refuse unexplained
  ignored state using the same typed error.
- Policy source identity normalizes regular-file mode to executable bits so a
  clean checkout with a different umask reproduces the same verified content
  identity. Full snapshot comparison retains the complete observed mode for
  race and mutation detection.

## AW-31 Bounded Parallel Queue Workers (2026-07-12)

- Queue concurrency policy is strict `autonomous-queue-policy-v1`, included in
  `revolvr-effective-run-config-v5`, defaults to one worker, and has a fixed
  harness cap of four. CLI override is `run --queue/--daemon --workers N`; the
  TUI remains explicitly sequential.
- `internal/autonomousqueue` owns durable batch admission and reconciliation,
  not task lifecycle work. `autonomous-queue-operation-v2` persists a complete
  ordered batch before launch, with monotonic selection sequence, batch/slot,
  exact scheduling fingerprint/authority, task and task-operation identities,
  slot state/outcome, worker bound, peak, batches, and fallback reason.
- Admission retains AW-23 authority: start with canonical ready order and
  re-run the exact snapshot classifier against the occupied IDs after every
  candidate. Dependency/conflict uncertainty never broadens overlap. A missing
  classifier or no provably safe next candidate reduces the batch and records
  a typed sequential fallback.
- Task-operation IDs hash queue operation, selection, batch, slot, task,
  scheduling fingerprint, and worker bound. Stable IDs and result order never
  use goroutine identity, PID, map order, completion timing, or ambient time.
- Workers receive child contexts and exact persisted slot material. The
  coordinator owns queue progress/checkpoint/history/ledger mutation and
  reconciles returned evidence in slot order. Safe local terminal-for-now
  results do not cancel peers. Global cancellation cancels and joins all
  workers. Safety preserves the first canonical slot's authority after all
  already-admitted peers reach their task-run boundary. Panic/ambiguous/runner
  failures become unsafe slot evidence and prevent new admission.
- Restart runs only admitted unresolved slots with their original AW-22 IDs;
  terminal slots and outcomes are never duplicated. Existing v1 sequential
  operations remain readable/replayable and nonterminal pinned selections
  migrate forward at one worker. Queue ledger v3 adds real concurrency facts;
  v2 sequential evidence remains decodable/exportable and is not reinterpreted.
- Lock order is: outer autonomous-execution lease; queue operation lease; then
  worker-owned existing boundaries as needed (Git-admin before task state,
  child publication, or workspace source writer according to their owners).
  The coordinator holds no shared mutex across model, verification, or source
  work. Queue checkpoint/ledger publication happens after worker joins and is
  coordinator-only. Independent SQLite stores use their existing WAL/busy
  ownership; Git/ref/state effects retain existing explicit locks.
- Archive/reopen mutation now nonblockingly takes the outer execution lease
  before Git-admin and task-state, refusing while a direct run or queue is
  active. Retention retains its existing retention lease then nonwaiting outer
  execution probe. Queue execution never implicitly archives or retains.
- Notifications remain post-lease, one terminal queue adaptation with no
  worker dispatch. AW-30 metrics add configured/peak workers, parallel sweeps,
  batches, and fallbacks from queue ledger v3; older queue evidence produces an
  explicit concurrency omission. CLI/TUI render only durable attribution.

## AW-30 Metrics And Deterministic Evaluations (2026-07-12)

- Pure metrics projection lives in isolated `internal/autonomousmetrics` with
  schema `autonomous-loop-metrics-v1`. Its public contract is strict,
  map-free, deterministically ordered, caller-owned, canonical JSON with one
  final newline, and contains source references plus explicit omissions.
- Both sources adapt to `ledger.Snapshot`. Live reads use immutable read-only
  SQLite; `ledgerexport.ReplaySnapshot` verifies the immutable export before
  rebuilding the same logical runs/events. Canonical source identity hashes
  logical evidence rather than its live/export location, so identical history
  produces identical metric bytes.
- Logical terminal task operation ID, attempt ID/kind, audit run, verification
  run/occurrence, archive ID, finalization operation, and queue operation are
  the deduplication keys. Exact duplicates/replay are ignored; materially
  conflicting reuse fails. `no_task` is not a terminal task operation.
- Success numerator is completed terminal task operations; denominator is all
  counted terminal task operations. Raw stop authority is retained while
  rendering completed, blocked, needs-input, safety, budget, no-progress,
  cancelled, max-cycle, and unsafe classes distinctly.
- Actual AW-15 completion tokens and nanosecond durations are counted only when
  recorded. Missing token values are explicit and never estimated. Ledger run,
  task-operation, verification, and queue durations use recorded stable units
  and no ambient `now`.
- Audits are keyed by exact audit run. Findings are introduced once by exact
  report evidence and joined only to explicit durable resolution dispositions;
  a later clean audit never manufactures resolution. Verification ordinary
  attempts, failures/passes, flaky classifications, reruns, timeout,
  cancellation, missing-command, and runner-error facts remain distinct.
- Task-run/queue/archive/finalization owner payloads advance compatibly to v2
  where exact timing/statistics were absent. Tiered verification ledger events
  gain `autonomous-verification-ledger-event-v1`. Relevant older payloads yield
  typed omissions; unknown current schemas/fields/enums fail rather than being
  guessed.
- Archive latency is recorded only from the archive owner's exact matching
  terminal/archive UTC authority. Queue throughput is tasks run divided by
  durable queue nanoseconds, with sweeps, selections, ordered outcomes, stop
  reasons, and drained counts. AW-30 defines no concurrency metric or worker.
- App owns source loading, export verification/replay, configured-secret
  refusal, and pure projection invocation. CLI owns stable `metrics show`,
  `--json`, and `--export` rendering. The path is read-only and cannot dispatch
  notifications, run archive verification, mutate retention, execute workflow
  work, invoke Git/Codex, or populate ordinary status/TUI views.
- The source-of-truth evaluation suite is fixed-time typed fake evidence in
  `internal/autonomousmetrics`; it composes production owner contracts and the
  pure projector without duplicating production execution. It covers exactly
  the eight AW-30 scenarios. Existing owner failure-injection tests remain the
  crash/persistence authority, and live dogfood remains separate and opt-in.
- AW-31 remains the sole owner of bounded parallel workers, overlap analysis,
  concurrency configuration, and parallel scheduling.

## AW-29 External Notification Hooks (2026-07-12)

- Notification delivery is isolated in `internal/autonomousnotification`.
  Canonical source owners remain authoritative; notification code cannot mutate
  tasks, state, questions, queues, daemons, finalization, archives, Git, ledger,
  retention, or scheduling and cannot invoke a model or recurse into itself.
- The closed event enum is exactly `task_completed`, `task_blocked`,
  `task_needs_input`, `safety_stop`, `queue_drained`, and `daemon_failed`.
  Unknown events fail. Task and queue events require exact terminal durable
  operations; typed input additionally requires its AW-17 question history;
  daemon failure requires a durable queue occurrence. Ordinary errors and prose
  never become safety, input, completion, or failure authority.
- Payload schema is `revolvr-notification-payload-v1`: compact canonical JSON
  plus one newline, map-free fields, deterministic event/delivery IDs and UTC
  occurrence, repository/config/policy identities, typed outcome facts, fixed
  reference applicability, sorted omissions, and bounded redacted detail. A
  task completion does not fabricate or verify an archive reference.
- Policy schema is `revolvr-notification-policy-v1`, disabled by default, and
  advances the effective identity to `revolvr-effective-run-config-v4`.
  Enabled authority is a strict event allowlist, one exact executable and
  ordered argv, fixed repository-root working directory, ordered replacement
  environment names, timeout/caps, positive bounded attempts, and bounded
  retry delay. Policy normalization clones caller slices and canonically orders
  events without changing argv/environment order.
- Hook environment values are loaded only at delivery and every environment
  name must be covered by AW-19 redaction. Hooks inherit no ambient environment.
  Configured values and policy material containing them are refused/redacted;
  only names and nonsecret bounds enter config identity and operator output.
- Delivery identity hashes the exact durable source occurrence, event, complete
  redacted payload material, effective config, and hook policy. Storage is
  `.revolvr/autonomous/notifications/<delivery-id>/` with immutable intent and
  payload, immutable history-before-journal transitions, strict readback,
  content conflict refusal, safe path/link/mode checks, and an operation lock.
- Intent and exact payload are durable before invocation. Attempts record exact
  bounded authority, timing, exit/timeout/cancellation/runner classification,
  redacted capped output/error, truncation, retryability, and terminal stage.
  Nonzero/timeout failures alone retry; attempts/delay are bounded and
  cancellation-aware. Valid history can reconstruct a missing/lagging journal;
  interrupted running work consumes an attempt before any restart retry.
- Success is terminal locally. Exact replay after success starts no process;
  terminal failure remains inspectable and warns on replay. Revolvr does not
  promise external exactly-once behavior across a crash after receiver action
  but before synchronized local success; identical retries carry one stable
  receiver idempotency key.
- App adapters run only after the global autonomous-execution lease is released.
  Queue-internal task execution does not notify; the enclosing queue adapts its
  durable ordered task outcomes once. No source/state/Git/admin/child/archive/
  retention lock is held during external execution. Hook failure is persisted
  and observed but source result/error precedence and every lifecycle byte stay
  unchanged.
- `notification list/show` is the only new operator management surface. It is
  redacted, bounded, and read-only; it cannot resend, edit, delete, dispatch, or
  retry. Notification outbox evidence is not inserted into ledger/export or
  AW-25 retention candidates, so those versioned owners require no schema or
  pin-closure change.
- Disabled hooks do no executable lookup, environment load, outbox/lock/temp/
  ledger creation, or process execution during task/queue/daemon work. AW-27
  views and AW-28 TUI refreshes remain read-only and never dispatch.
- AW-29 adds no metric/evaluation projection and no concurrency. AW-30 and AW-31
  retain those respective boundaries.

## AW-28 TUI Autonomous Workflow Visibility And Controls (2026-07-12)

- The TUI's rich workflow detail consumes only `autonomousview.View`; it does
  not own a parallel evidence schema, parse autonomous JSON, infer supervisor
  decisions, or open task/state/archive/ledger stores. App callbacks remain
  the only repository/runtime boundary and CLI only wires those callbacks.
- Workflow is an additive `6` view. Existing Dashboard/Tasks/Runs/Run Detail/
  Preflight/Help rendering and number keys remain compatible. A bounded app
  selector summary joins strict canonical active-task discovery with strict
  archive-manifest discovery so active, archive-only, empty, and mixed-pass
  repositories remain navigable without directory crawling in the TUI.
- Selector controls stay separate from display strings. Exact task/archive IDs
  remain callback authority in model state, while app-redacted labels and the
  redacted AW-27 view are the only rendered identities. Selector discovery is
  observational and never loads state, verifies an archive, creates a cache,
  or opens writable runtime evidence.
- Selector and detail loads are asynchronous Bubble Tea commands identified by
  exact selector plus a monotonic model token. Stale responses cannot replace
  current evidence. Refresh and operation completion reload deliberately, task
  selection is preserved by task ID, and active-to-archive movement follows
  that identity rather than retaining an obsolete list index.
- Workflow rendering uses stable plain-text sections before Lip Gloss styling.
  The existing width-aware wrapper and shared viewport own narrow rendering;
  page/top/bottom keys own long-detail scrolling. Statuses, dispositions,
  budgets, gate facts, archive verification state, and terminal reasons are
  textual rather than color-only.
- Structured input mutation belongs in app `AnswerAutonomousInput`, which
  reloads exact current active task/state/question/option authority, uses AW-17
  CAS operations to persist an explicit operator answer and then resume, and
  redacts every returned error. A persisted answer plus failed resume is a
  typed partial success; retry recognizes and resumes the same exact answer
  without recording a duplicate. The TUI never edits state/task files.
- Input recommendations are evidence only. The Workflow answer control starts
  with selection `-1`, displays every option and recommendation, requires an
  explicit option movement, then requires a separate confirmation enter. Only
  a current typed waiting question on an active task is answerable; legacy,
  archive, stale, missing, answered-with-another-option, or absent authority
  fails closed.
- Task `U` and queue `Q` share the established preflight and one-active-TUI-
  operation state. App/domain revalidation and the global autonomous execution
  lease remain authoritative. Queue defaults match CLI policy (100 tasks, 50
  cycles), uses `app.RunQueue`/AW-24 rather than a model loop, streams typed
  progress/outcomes, and retains replay/stop evidence.
- `c` cancellation only cancels the context created for the current
  TUI-started mixed pass, loop, task run, or queue. Repeated cancellation is
  idempotent, new starts remain refused until the domain returns its typed
  terminal result, and no lifecycle/task/workspace is synthesized or deleted
  by the model.
- Workflow archive reads use strict AW-27 Show and remain explicitly
  `unverified`; AW-28 never invokes archive Verify/create/reopen merely to
  render. It also adds no daemon-service control, notification, metric/
  evaluation, retention action, workspace cleanup, model invocation, network
  request, or parallel worker. AW-29 through AW-31 retain those boundaries.

## AW-27 Read-Only Autonomous Evidence Views (2026-07-12)

- Pure operator projection belongs in isolated `internal/autonomousview`, not
  app orchestration, Cobra callbacks, status/timeline strings, the TUI, or the
  dossier cache. Schema `autonomous-task-view-v1` is strict, map-free,
  deterministic, deep-clones caller evidence, and performs no I/O.
- The app owns exact selector resolution and read-only evidence assembly.
  `ShowAutonomousTask` considers exact active task IDs plus archive/task IDs,
  rejects active/archive and multi-archive ambiguity, and never silently
  chooses one authority.
- Active authority is the duplicate-checked canonical task plus its exact
  `autonomous_state_path` loaded through `autonomousstate`. Optional committed
  audits and the bounded latest decision artifact may enrich the view, but
  task/state identities are reloaded after collection and any changing
  canonical snapshot fails closed.
- `autonomousarchive.LoadEvidence` is the archive-side strict Show extension:
  it reads exact archived task, terminal state, and applicable frozen
  completion evidence by manifest identity. It deliberately does not perform
  full archive Verify and returns no `verified now` claim. Non-completed
  dispositions render AW-20 completion evidence as not applicable.
- Required malformed task/state/archive authority is a hard read failure.
  Optional malformed audit history becomes a typed omission while other valid
  state renders. Malformed latest accepted decision payload makes admission
  unknown; reference presence alone never authorizes a route. Unknown future
  schemas/enums remain strict failures at their canonical owners.
- Plans retain plan ID/revision/predecessor/completed status, source order,
  step status/description/rationale, and typed evidence. Acceptance keeps
  pending/satisfied/waived/not-applicable distinct with rationale/evidence.
  Findings join committed audit identities with durable resolution state and
  never disappear merely because the newest optional history is unavailable.
- Attempts expose total/per-action counts, consecutive failures, exact event
  references, stops/circuit breakers, and distinct unset/limited/unlimited
  retry/elapsed/token/action budgets. Limited remaining values clamp display at
  zero while exhausted/over-limit authority remains explicit; token usage is
  never estimated from dossier size.
- Typed input displays the exact question revision/content identity, ordered
  options, and recommendation as evidence only; it never selects an answer.
  Durable answers retain answer/option/actor provenance, and legacy reason-only
  needs-input remains explicitly non-answerable.
- Why output has four separate fields: latest accepted decision, currently
  admitted action, scheduler readiness, and next supervisor action. Durable
  lifecycle/decision agreement is required for current admission. Otherwise
  `next_supervisor_action` is `undetermined_requires_supervisor`; no plan step,
  lifecycle name, profile, task prose, receipt, or cache entry predicts it.
  Reason ordering is explicit: terminal, finalizing, input, blocked,
  budget/breaker, scheduler, plan/acceptance/finding/verification/audit gates,
  then undetermined-next.
- Ordinary task viewing does not run archive Verify merely to satisfy a
  dependency. Active dependencies may be explained directly; an archived
  dependency remains `archive_dependency_unverified` until the separate
  verification boundary supplies scheduler authority. Views never invent
  conflict occupancy.
- CLI owns rendering through exactly `task show` and `task why`; no per-field
  command family was added. Human output is stable plain text with explicit
  absence/mode words and UTC RFC3339Nano times. `task show --json` is the
  canonical validated projection, not an alternate untyped payload.
- AW-19 redaction remains harness-owned. App loads configured secret values,
  projects typed evidence, then redacts every string in the cloned result and
  all errors before CLI rendering. Secret values never enter human/JSON output,
  diagnostic references, or help fixtures.
- AW-27 viewing creates no ledger, WAL, lock, cache, journal, receipt, export,
  or repair sidecar and acquires no mutation lease. It does not run Git, Codex,
  verification, audit, archive verification, workspace recovery, retention,
  notification, metrics, or parallel workers. AW-26 dossier cache remains
  completely noninteracting.
- Existing task list/status/run show/archive/config/doctor/receipt/TUI output is
  preserved. AW-28 remains the sole owner of TUI evidence screens and controls;
  AW-29 through AW-31 remain notifications, metrics/evaluations, and bounded
  parallelism respectively.

## AW-26 Dossier Cache And Role Projection (2026-07-12)

- Immutable dossier-context caching lives in isolated `internal/dossiercache`,
  never in prompt construction, cycle routing, CLI/TUI, ledger, or retention.
  Namespace/schema are `.revolvr/cache/dossier/v1/<key>/` and
  `revolvr-dossier-cache-v1`; producer algorithm is
  `git-tree-path-map-v1`. Entries are immutable `manifest.json` plus
  `repository-map.md` with synchronized temporary-directory publication.
- A cache key is canonical JSON SHA-256 over schema/algorithm, hashed canonical
  control and execution roots, exact commit/tree IDs, map path/byte bounds, and
  ordered applicable guidance path/SHA-256/byte-size identities. No task ID,
  clock, PID, random value, mtime, cache counter, model output, receipt,
  environment value, or configured secret value is key authority.
- Only exact committed-tree repository-map derivation is cached. Task/spec,
  state, plan, acceptance, findings, input, attempts/budgets, workspace,
  verification, audit, receipts, history, worktree status/diff, profiles,
  safety/config, schemas, routes, and prompts are live. Same reusable map bytes
  therefore cannot carry task/profile/role control evidence across calls.
- Repository mapping consumes only bounded `rev-parse`/`ls-tree -r -z` object
  metadata for the exact commit. It sorts canonical tracked paths, excludes
  `.git`/`.revolvr`, labels conservative file kinds and symlink/submodule
  metadata, and truncates only on whole paths with explicit counts. It runs no
  build, hook, test, container, network request, model, language server, or
  repository binary.
- Cache lookup is strict and read-only. Unsupported/noncanonical JSON,
  unknown fields, source/key/hash/size/count/token disagreement, symlinks,
  hard links, unsafe modes, oversize, partial publication, or unsafe parents
  never yield content. A valid hit is byte-equivalent to recomputation.
  Corruption/unsupported evidence causes safe exact Git recomputation and a
  deterministic diagnostic in run provenance; corrupt evidence is neither
  deleted nor overwritten and never becomes workflow authority.
- Pure role projection remains in `internal/autonomous` and supports exactly
  `supervisor`, `planner`, `implementer`, `auditor`, `corrector`,
  `documentor`, and `simplifier`. Existing validated action/profile pairs map
  to one role; contradictions/future roles fail closed. Schema
  `autonomous-role-dossier-manifest-v1` and policy
  `role-section-matrix-v1` explicitly record every included/omitted section.
- Every role receives exact task, current state, plan, acceptance, Git source,
  guidance, and repository map. Supervisor additionally receives all routing
  gates/history; planner omits current verification/audit; implementer omits
  audit/history; auditor/corrector/documentor/simplifier receive current
  verification and audit/finding authority but omit unrelated run history.
  Exact correction/verification authority continues in the route-specific
  worker prompt. Omission never grants authority.
- Section manifests record exact total/included bytes, whole item counts,
  omission reasons, and deterministic `utf8-bytes-ceil-div-4-v1` estimates;
  final dossier and prompt identities/sizes/estimates are separate. Estimates
  are explicitly not actual Codex usage; receipts/AW-15 remain actual token
  accounting authority.
- Cache use never weakens source freshness. Full content-sensitive source
  snapshots still bracket assembly/supervision and worker admission; assembly
  also reloads task/guidance bytes after cache collection. Changed HEAD/tree,
  guidance, root/worktree, task/state/profile/config/safety, or source evidence
  cannot be admitted from cache presence.
- Every invocation retains an exact per-run dossier and manifest in addition
  to the existing exact prompt/profile/schema/output/provenance artifacts:
  `supervisor-dossier.md`, `supervisor-dossier-manifest.json`,
  `worker-dossier.md`, and `worker-dossier-manifest.json`. Ledger extraction
  and export know these paths. Run provenance binds cache result/key/entry
  manifest, role/policy, dossier, prompt, task/source/profile/config/safety,
  and run identities; it never points only to shared cache bytes.
- AW-25 retention owns only its existing ledger-derived stream candidates and
  ignores `.revolvr/cache`; AW-26 adds no eviction. Ordinary status, show,
  config, doctor, TUI, archive verification, and GC do not populate or repair
  cache entries. No new CLI/TUI evidence screen or configurable budget was
  added; fixed bounded harness policy is sufficient for this slice.
- AW-27 remains the owner of broad app/CLI evidence and why-next projections;
  AW-28 owns TUI workflow controls; AW-29 notifications, AW-30 metrics and
  evaluations, and AW-31 parallel workers remain out of scope.

- The repo itself is the durable state for autonomous loop runs.
- AW-25 policy schema is `revolvr-artifact-retention-policy-v1`; it is strict,
  map-free, harness-authored, and part of `revolvr-effective-run-config-v3`.
  Defaults are mutation disabled, 20 recent runs pinned, seven-day compression,
  90-day prune age, 64 KiB compression threshold, Codex JSONL/stderr classes,
  pruning disabled, verified export required for pruning, 100 files/1 GiB per
  operation, and 256 MiB decompression cap. Secret values and clocks/callbacks
  never enter this identity.
- `internal/artifactretention` exclusively owns candidate inventory, pin
  closure, deterministic plans, compression, GC quarantine, journals, and the
  global retention lease. Candidates come only from exact ledger-owned Codex
  JSONL/stderr paths; tracked tasks/archives, canonical state/history,
  completion, queue/task-run/child/workspace/control files, locks, source, Git,
  and unknown files are never generic age targets.
- Pinning is conservative and transitive over both exact artifact paths and run
  identities found in strict autonomous/archive/recovery JSON. Every active
  task pins its run history; nonterminal/recent runs pin directly; incomplete
  operations pin recovery evidence; cleaned GC history does not permanently
  pin. Missing, unsafe, ambiguous, foreign, symlinked, hard-linked, or changed
  evidence blocks rather than widening deletion authority.
- Compression format is standard-library deterministic gzip with zero/fixed
  headers and adjacent canonical `revolvr-compressed-artifact-v1` manifest.
  Logical identity remains the original path/SHA-256/size. Readers accept
  exactly one original or admitted compressed representation and reject dual,
  corrupt, divergent, oversized, or partial forms. An original must first be
  durably compressed; only a later operation may prune it.
- `internal/ledgerexport` owns schema `revolvr-ledger-export-v1` plus canonical
  JSONL record schema `revolvr-ledger-export-record-v1`. Exports bind immutable
  read-only SQLite identity and high-water range, every run field, global event
  IDs/order/times/types, exact payload bytes, explicit legacy/versioned schema,
  predecessor coverage, and compressed artifact facts. Live ledger rows remain
  authoritative and are not deleted by AW-25.
- Export verification is read-only and checks canonical manifests/records,
  hashes/sizes/counts/range gaps/order, source-ledger coverage, predecessor,
  payload schema agreement, compressed facts, and configured-secret absence.
  Replay reconstructs logical run/event terminal evidence in memory and never
  replaces the ledger, executes work, invokes Git/Codex, or claims pruned source
  artifact bytes were recreated.
- GC planning is the canonical dry-run and must create nothing. A plan binds
  one frozen UTC time, operation/policy/config/repository/ledger/high-water,
  exact inventory/pins, ordered retain/compress/prune actions and expected
  identities/bytes, limits/remaining work, export requirement, and a
  content-derived plan identity. Apply never silently replans a different
  action set and requires the exact printed plan identity.
- GC durability schema is `revolvr-artifact-gc-journal-v1` under
  `.revolvr/retention/gc/<sha256(operation-id)>/`. Synchronized immutable
  history precedes mutable journal replacement. Stage order is admission,
  verified/replay-validated export, deterministic compression, operation-owned
  quarantine, completion, cleanup, then cleaned. Cancellation starts no new
  action and records resumable authority; retries reconcile exact orphan/dual/
  quarantine effects and conflict on changed bytes.
- Retention lease order is outermost, then the existing autonomous-execution
  lease; Git-admin and child-publication locks are only probed and never held
  during scans/export/compression, and live source writers refuse admission.
  Ledger identity/high-water and active/recent/control pins are revalidated
  before every mutation. Ordinary run/status/show/config/doctor/TUI/archive/
  queue operations never trigger retention work.
- App and CLI expose only narrow plan/apply/resume/inspect and ledger
  export/verify/replay operations. `artifact gc` defaults to dry-run and apply
  needs `--apply --plan-id`; there is no `--force`, arbitrary deletion root,
  implicit apply time, broad evidence screen, or TUI control in AW-25.
- AW-24 queue ownership lives in isolated `internal/autonomousqueue`, above the
  pure AW-23 scheduler and exact AW-22 task runner. It owns repeated fresh
  selection, queue-local fairness, durable in-flight recovery, ordered task
  outcomes/statistics, bounds, summaries, and queue stop classification; it
  never performs task effects, archive creation, input answers, cleanup, or
  parallel execution.
- Queue schema `autonomous-queue-operation-v1` lives under
  `.revolvr/autonomous/queues/<operation-id>/`. It binds mode, effective config,
  safety declaration, task bound, sweep/time/sequence/stage, exact scheduling
  fingerprints, deterministic derived AW-22 operation IDs, authority-bound
  exclusions, outcomes, remaining work, and terminal reason. Immutable
  synchronized history precedes atomic checkpoint replacement; exact terminal
  replay starts no task and changed material conflicts.
- Queue fairness never changes canonical scheduling metadata. A safe yielded
  task is excluded only while the SHA-256 authority over its exact task bytes,
  state identity, status, and lifecycle remains unchanged. Fresh deterministic
  scheduler order still governs every nonexcluded ready candidate.
- Queue stop precedence is input waiting, blocked-only waiting, dependency or
  yielded waiting, then drained when no active pending autonomous work remains.
  Queue-owned task bounds produce `budget_exhausted` with remaining-ready
  evidence. Caller cancellation, trusted safety stops, and unsafe/ambiguous
  evidence remain distinct and stop all new starts.
- AW-24 derives each AW-22 ID from queue ID, monotonic selection number, task
  ID, and exact preselection scheduling fingerprint, persists it before start,
  and always reopens that identity after interruption. AW-22 remains a pinned
  single-task primitive and never sees surplus queue budget.
- The global `.revolvr/locks/autonomous-execution.lock` is owned by
  `internal/autonomousexec` at the app/coordinator boundary. Direct AW-22 runs
  and bounded queue sweeps acquire it outermost; the queue calls an explicit
  already-leased app path. Git-admin, task-state, child-publication, workspace
  source-writer, and AW-22 operation owners never acquire it, preventing lock
  inversion and self-deadlock.
- `internal/autonomousdaemon` owns only foreground polling, exact pre/post-sweep
  authority comparison, stable debounce, wake observation, and cancellation.
  It holds no execution/source/state/Git/child lock while waiting and requires
  explicit AW-19 `fully_unattended` authority. The fingerprint covers active
  task bytes, exact autonomous state identities, verified archive authority,
  and completed child publication while excluding queue/ledger self-effects,
  mtimes, and timestamps.
- Queue ledger evidence uses one deterministic run and versioned admitted,
  selection, task-stopped, daemon-wake, and stopped events with exact content
  comparison. The run completes only after the terminal queue checkpoint is
  readable. Wake count/fingerprint are bounded to the next daemon sweep rather
  than retaining unbounded raw filesystem events.
- The AW-24 CLI surface is `run --queue` and foreground `run --daemon`, mutually
  exclusive with mixed-pass and exact-task modes. App owns assembly and typed
  operations; CLI owns validation/rendering. Current TUI behavior is preserved,
  broad controls remain AW-28, and queue concurrency remains exactly one until
  AW-31.
- AW-22 exact-task orchestration lives in `internal/autonomoustaskrun`; its
  operation lease is separate from Git-admin, state, and source-writer locks,
  and no other owner may acquire it, so bounded effect owners can retain their
  established lock orders without inversion.
- `autonomouscycle` exposes one nil-compatible pre-worker admission callback
  only after the route and admission source are validated. AW-15 admission is
  performed there; worker execution is never charged after it starts.
- AW-22 pins one active `autonomous-v1` task per versioned durable operation
  under `.revolvr/autonomous/task-runs/<operation-id>/`; immutable history is
  authoritative when it is newer than the synchronized mutable checkpoint,
  and a separate operation lease never nests inside Git-admin, state, or
  workspace source-writer locks.
- Attempt-consuming correction/document/simplify decisions use the same AW-10
  pre-worker admission seam as ordinary work. The already-admitted one-shot
  result is then continued only by `autonomouscorrection` or
  `autonomousoptional`, which retain final verification, finding resolution,
  fresh re-audit, attempt completion, and optional occurrence ownership.
- An explicit supervisor block is a canonical one-way transition owned by
  `internal/autonomousblock`: it re-evaluates the pure route, writes immutable
  `autonomous-block-transition-v1` history before exact state CAS, and
  publishes blocked task status without running a worker or charging an
  attempt.
- Autonomous task-run ledger summaries use one deterministic operation run
  plus versioned admitted/cycle-started/cycle-completed/restarted/stopped
  events. Exact retry deduplicates those summaries; completed loop evidence is
  written only after the bounded terminal owner has succeeded.
- AW-22 completion is terminal-but-unarchived. The loop invokes AW-20 for a
  validated `complete` route but never infers AW-21 archive time, provenance,
  or administrative-commit authority; archive remains a separate command.
- Each loop pass starts as a fresh `codex exec` session.
- Do not use resumed Codex sessions for this workflow.
- Work is limited to one small, verifiable task per pass.
- Existing project patterns should win over new abstractions unless a task explicitly asks for a change.
- Tests, build output, and typechecks are the source of truth.
- `.revolvr/` is local runtime state for the Revolvr CLI and is ignored by Git.
- CLI-initiated harness runs default to Codex dangerous bypass/yolo mode for unattended autonomy; repo config can disable it with `codex.dangerously_bypass_approvals_and_sandbox: false` or `codex.yolo: false`.
- `revolvr init` keeps runtime state local by adding `/.revolvr/` to `.git/info/exclude` when initialized from a Git worktree, avoiding tracked `.gitignore` changes.
- Dogfood readiness is exposed as a top-level `revolvr doctor` command so preflight checks stay separate from config inspection.
- `revolvr run` streams summarized Codex progress events to stdout while keeping raw Codex JSONL and stderr as run artifacts; console output should stay human-readable and artifact-backed.
- `revolvr receipt validate <run-id>` treats the ledger run row plus run events as the source of truth for finalized receipt validation, and recorded artifact paths must exist on disk.
- `revolvr task retry <task-id>` is the preferred blocked-task recovery command; it reuses the same blocked-to-pending task update as `task unblock` so task identity and prior run history remain intact.
- `revolvr run --max-passes` must print one concise final loop summary and stop before a failed dirty pass or blocked outcome can cascade into blocking unrelated tasks; only clean failed outcomes may repeat, and those stop after two consecutive failures.
- The real-Codex live dogfood check is an opt-in script (`scripts/dogfood-live.sh`) because it removes local `.revolvr/` runtime state and creates a Git commit; it must require a clean source worktree before resetting state.
- Read-only CLI inspection should move orchestration into `internal/app` while keeping exact command rendering in `internal/cli`; `status` and `show` are the first commands following this boundary.
- Receipt validation orchestration now follows the same boundary: `internal/app` resolves state, loads run history, and invokes receipt validation, while `internal/cli` keeps exact command rendering and failed-check exit formatting.
- Task command orchestration follows the same boundary: `internal/app` owns task add/list/retry/unblock state resolution, task-file access, and blocked-to-pending transitions, while `internal/cli` keeps the exact command output formatting.
- Run orchestration follows the same boundary: `internal/app` owns run config loading, single-pass execution, loop stats, stop reasons, outcome errors, and `run --max-passes` guardrails; `internal/cli` keeps exact run summary, progress, and loop summary rendering through callbacks.
- Charm TUI dependencies are pinned to the newest stable tagged releases that preserve the repository's Go 1.22 module baseline: Bubble Tea `v1.3.4`, Bubbles `v0.20.0`, and Lip Gloss `v1.1.0`. Newer Bubble Tea and Bubbles tags require Go 1.23 or 1.24, so they are intentionally avoided until the project raises its Go floor.
- `revolvr tui` follows the app boundary: `internal/cli` loads `internal/app.Status` and passes that snapshot to `internal/tui`, while `internal/tui` owns Bubble Tea program execution and rendering.
- TUI run controls follow the same app-boundary callback pattern as other TUI actions: `internal/cli` wires `RunOnce` to `internal/app.RunOnce`, while `internal/tui` owns readiness guards, one-active-run state, progress rendering, cancellation, and post-run refresh/detail loading.
- The TUI shell uses an explicit view model with number-key navigation: `1` Dashboard, `2` Tasks, `3` Runs, `4` Run Detail, and `?` Help. Switching views must preserve the loaded status snapshot, selected run, and opened run detail unless state is refreshed to uninitialized.
- TUI layout renders plain text first, wraps header/content/key-help lines to the current terminal width, and only then applies semantic Lip Gloss styles. Status words and markers such as `OK`, `FAIL`, `PASS`, `Status: failed`, and `! blocked` remain in the text so important states are readable without color; recent-run rows switch to a compact layout below 72 columns.
- The TUI Tasks view derives list rows and selected-task details from `internal/app.StatusResult.Tasks`, uses its own in-memory selection index, and makes blocked tasks visibly distinct with a text marker so the status is clear without relying on terminal color.
- TUI write actions use explicit model callbacks that are wired by `internal/cli` to `internal/app`; task creation calls `internal/app.AddTask`, then refreshes through `internal/app.Status`, switches to the Tasks view, and selects the added task from the refreshed snapshot when present.
- The TUI Runs and Run Detail views stay read-only and app-boundary-backed: run lists use `internal/app.Status` recent runs, detail opening uses `internal/app.ShowRun`, and detail diagnostics are derived from ledger events into separate Summary, Diagnostics, Changed Files, Artifacts, and Events sections. Missing artifact paths, missing changed-file events, receipt warnings, and long event lists must remain visible inside the TUI without requiring `revolvr show`.
- TUI receipt validation stays app-boundary-backed and manually triggered from Run Detail with `v`: `internal/cli` wires the callback to `internal/app.ValidateReceipt`, while `internal/tui` stores and renders the latest validation result for the loaded run with explicit `PASS`/`FAIL` check lines and non-crashing error display for missing or unreadable receipts.
- Doctor/preflight orchestration now belongs to `internal/app.Preflight`: it returns structured readiness checks while preserving the existing doctor check order and detail strings. `internal/cli` renders the established `revolvr doctor` output unchanged, and the TUI uses an app-wired Preflight callback for the `5 Preflight` view plus `p` reruns.
- Next-phase development treats Markdown task import as the bridge from external design/spec chat to Revolvr task files. Initially, acceptance and verification notes should be preserved in task text plus summary rather than expanding task frontmatter; add structured task fields only after the plain-text import flow proves insufficient.
- The initial Markdown task import parser lives in `internal/taskimport` and recognizes repeated `## Task` or `## Task: summary` sections, optional `### Summary`, `### Acceptance`, and `### Verification` subsections, and unknown subsections that are preserved in task text. It returns a local `{Task, Summary}` spec instead of importing `internal/app`, so the next app-level import operation can consume it without a package cycle.
- App-level task import is split into parser exposure and execution: `internal/app.ParseTaskImport` wraps `internal/taskimport`, `ImportTasksFromMarkdown` is the convenience parse-and-import path, and `ImportTasks` accepts already parsed tasks for dry-run/write modes. Dry-run and empty parsed imports must not create task files or `.revolvr/`; write mode validates every task before creating files so persisted list order matches input order.
- CLI task import stays thin and app-boundary-backed: `revolvr task import <path>` reads the Markdown file in `internal/cli`, delegates parsing/importing to `internal/app.ImportTasksFromMarkdown`, prints numbered dry-run rows from parsed task descriptions, and prints created task IDs in parsed order for write imports. Existing `task add` and `task list` rendering remains unchanged.
- README is the operator-facing reference for the chat-to-task import workflow: it documents the minimal Markdown import shape, dry-run/import commands, TUI refresh/preflight/run-once keys, and the rule to avoid concurrent code edits in the same worktree while a harness pass is running.
- TUI next-runnable visibility is derived from the existing `internal/app.StatusResult.Tasks` snapshot rather than new task-store fields: the Dashboard and Tasks view count pending, blocked, and completed tasks locally, treat the first pending task as the next runnable task, and render explicit `Runnable: ready to run` / `Runnable: nothing runnable` text plus a plain `next` list marker so the state is clear without color.
- TUI blocked-task retry follows the existing app-boundary callback pattern: `internal/cli` wires `internal/app.RetryTask` into `internal/tui`, while the TUI validates the selected task status before calling the callback, refreshes after successful retry, keeps the selected task stable by ID, and reports non-blocked selections, missing callbacks, retry errors, and refresh failures inline.
- Run profiles are repo-authored Markdown files under `.agent/profiles/<name>.md`, with `implementer` as the default profile name. Runtime fallback to hardcoded profile text is intentionally avoided; missing, empty, or unsafe profile files should block clearly before Codex starts. Embedded profile text may exist only as `revolvr init` seed/template content.
- Exact context payloads are retained as first-class run artifacts for audit and reuse: each run writes the full Markdown context payload plus `context.json` manifest before Codex starts. Future summaries or compression may be added as additional artifacts, but they must not replace the exact payload that was sent to Codex.
- `.revolvr/runs/<run-id>/context.md` and `.revolvr/runs/<run-id>/context.json` are the only supported run context artifacts; alternate context artifact names and ledger keys are not supported.
- `.agent/tasks/*.md` are the canonical human-authored task specs. Runtime initialization creates task-file directories and ledger state only.
- `internal/runonce` consumes only canonical `.agent/tasks/*.md` task files through `internal/taskfile`; when no pending file task exists, it returns `no_task`. It resolves the selected task's workflow and phase through `internal/passpolicy.Lookup`, loads the mapped repo-authored run profile, and records the selected workflow, phase, and profile name in the task-selected ledger event. Successful runs keep `selected_task` manifest metadata from the exact pre-run task bytes, then after Codex and verification pass they apply the policy transition before the commit gate: implement advances to pending audit only when pre-metadata changed files exist, audit/document/simplify may advance with no source changes when policy permits, and simplify completes the task. Changed files are recaptured after task-file metadata updates so commits and receipts include the durable transition. Blocking/failed outcomes mark selected task files `blocked` without advancing phase; if the commit gate fails after a metadata update, the task is blocked at its original phase. Task frontmatter `profile` remains reserved for a future slice.
- Mixed-pass workflow development treats a durable task and an execution pass as different concepts: one `.agent/tasks/*.md` task can move through multiple phases while each phase still runs as a fresh Codex session.
- Task workflow state belongs in the canonical task file. The initial workflow metadata shape is `workflow: mixed-pass-v1` plus `phase: implement|audit|document|simplify`, defaulting to `mixed-pass-v1` and `implement` for existing task files that omit the keys.
- Revolvr owns phase transitions through a pass-policy model, not through Codex-written task edits. Policy maps phases to profiles and outcome semantics: `implement` uses `implementer` and requires meaningful changes; `audit` uses `auditor`, `document` uses `documentor`, and `simplify` uses `simplifier`, with those non-implementation phases allowed to succeed without code changes when verification and receipts are otherwise acceptable.
- The pass-policy model lives in `internal/passpolicy`. `Lookup(workflow, phase)` is the integration point for later runtime slices and returns profile name, no-change success permission, next phase, and terminal completion state while reusing `internal/taskfile` workflow and phase constants.
- Successful phase advancement must be durable and auditable by updating the selected task file before the commit gate when a commit is appropriate. A task should stay pending while more phases remain and should only become completed after the final successful phase.
- Auto-commit outcome classification uses pre/post-commit HEAD evidence, not the commit command exit alone. Revolvr captures HEAD before staging, resolves it after every commit attempt, retries one failed post-commit lookup, and records an advanced HEAD as committed with its SHA. If post-commit HEAD remains unavailable, the result is explicitly `indeterminate`; runonce blocks and restages the transitioned task snapshot rather than restoring original-phase bytes that may already have been committed.
- App-level read visibility for tasks now comes from canonical `.agent/tasks/*.md` files through `internal/taskfile`: `internal/app.ListTasks` and `internal/app.Status(...).Tasks` adapt each `taskfile.Task` into the `internal/taskmodel.Task` rendering shape using task-file ID, full Markdown context body, H1 title as summary, and preserved task-file status. The adapter resolves workflow and phase through `internal/passpolicy.Lookup` and exposes the mapped run profile plus next phase or `completed`; CLI and TUI renderers consume those display fields without interpreting policy. Ledger/recent-run loading in `app.Status` remains unchanged.
- App-level task write operations also use canonical `.agent/tasks/*.md` files. `internal/app.AddTask` and write-mode `ImportTasks` create pending Markdown task files with generated IDs, `status: pending` frontmatter, H1 titles derived from summary or task text, and task text as the body; dry-run imports and validation/parse failures must not mutate `.agent/tasks/` or `.revolvr/`. `RetryTask` and `UnblockTask` find file-backed tasks by ID and only update `blocked` tasks to `pending`.
- Run timeline projection starts at the app boundary: `internal/app.RunTimeline` converts `ledger.RunWithEvents` into ordered `RunTimelineRow` values with timestamp, phase, status, and concise detail while keeping CLI/TUI rendering for a later slice. Event rows preserve ledger order and `CreatedAt`; missing start/terminal history falls back to the ledger run row, and malformed payloads produce generic rows instead of panics.
- Run timeline rendering is user-facing in both read surfaces: `revolvr show <run-id>` prints a tab-delimited `Timeline:` table immediately after run summary fields and before artifacts/diagnostics/events, while TUI Run Detail prints compact single-line timeline rows after `Summary` and before `Diagnostics`. Both surfaces use `internal/app.RunTimeline`; missing timeline row timestamps render as `none`, and raw TUI `Events` remain visible below artifacts for audit/debug detail.
- TUI bounded multi-pass runs use the existing app-boundary callback pattern: `internal/cli` wires `internal/tui.RunLoop` to `internal/app.RunLoop` with the same configured single-pass runner, while the TUI owns readiness checks, one-active-run state, cancellation, progress rendering, and post-run refresh/detail opening. The durable keybindings are `R` for single pass, `n` to cycle loop max passes through 2/3/5 with default 3, and `L` to start the bounded loop.
- Future `autonomous-v1` model output contracts live in `internal/autonomous` as plain JSON-tagged structs whose `Validate` methods apply Revolvr-owned policy before the output may drive work. Worker actions select one exact role profile (`planner`, `implementer`, `auditor`, `corrector`, `documentor`, or `simplifier`); `complete` and `block` are terminal and select none. Decisions and audits cite typed durable evidence with a reference and concrete detail. Audit findings use explicit `blocking`/`non_blocking` significance plus lower-case kebab-case IDs, and correction decisions are checked against the findings in a validated `changes_required` audit report. This package is not wired into `mixed-pass-v1` runtime behavior.
- Durable `autonomous-v1` execution snapshots use schema identity `autonomous-execution-state-v1`. Each plan revision has a new lower-case kebab-case ID, a monotonic revision number, and an explicit predecessor link; ordered step slices are canonical. Acceptance and finding dispositions reuse AW-01 evidence and finding-ID contracts, while compact decision references reuse AW-01 action/profile compatibility and retain decision, run, artifact, task, and timestamp provenance. Retry/count and elapsed-time budgets use explicit `unset`, `limited`, or `unlimited` modes, with durations encoded as nanoseconds and exact over-limit consumption retained for later AW-15 policy. Contract transition validation protects stable identities and irreversible terminal states only; routing, evidence freshness, completion gates, persistence, and explicit reopen policy remain outside AW-02.
- Autonomous task dossiers are a pure projection in `internal/autonomous`: callers supply exact task/guidance bytes and typed state, audit, verification, recent-run, and Git evidence; the builder performs no live discovery or I/O. Canonical Markdown section order is identity, task/spec, state, plan, acceptance, verification, audit/resolutions, recent runs, Git, guidance, then omission/truncation facts. Recent runs sort newest-first by supplied start time with run ID as the tie-breaker, and a zero history limit includes no runs.
- Task-dossier manifests use schema `autonomous-task-dossier-manifest-v1`. Raw task/guidance sources hash their exact supplied bytes; typed sources hash compact `encoding/json` bytes over structs or deterministically ordered slices; recent-history provenance always hashes the complete ordered history even when rendering is truncated. Manifest JSON is indented with a final newline, while its required hash describes only the final Markdown dossier to avoid recursive self-hashing.
- Direct `.agent/tasks/AGENTS.md` is reserved for applicable nested repository guidance and is not a task document. Every other direct `.md` file under `.agent/tasks/` remains subject to strict task parsing, duplicate-ID detection, and deterministic discovery.
- Autonomous dossier assembly lives in `internal/autonomousassembly` as a read-only boundary over `internal/taskfile`, immutable ledger access, exact receipts, bounded Git commands, and repository guidance. Selected-task history is filtered before its explicit collection limit and Git evidence is accepted only when HEAD remains stable across the complete collection window.
- Codex invocation configuration extends the shared app/runonce path with one canonical YAML shape: `codex.model`, `codex.reasoning_effort`, and `codex.ephemeral`. Missing keys default to `gpt-5.6-sol`, `xhigh`, and `true`; explicit empty model/effort values and non-ephemeral sessions are invalid, reasoning effort is limited to `minimal`, `low`, `medium`, `high`, or `xhigh`, and model validation stays structural rather than using a guessed allowlist.
- Effective run configuration provenance uses schema `revolvr-effective-run-config-v1`: SHA-256 covers compact `encoding/json` bytes for a map-free normalized projection of working directory, Codex/Git/verification/commit/source-lock settings and ordered command/argument slices. Process-local callbacks, stores, clocks, per-run identity, and artifact paths are deliberately excluded, and projection construction copies caller-owned slices.
- Canonical Codex invocation provenance is `codexexec.InvocationProvenance`, prepared before execution from the exact effective config and shared unchanged by `context.json`, `context_built`, and `codex_started`. It records the configured executable, bounded `<executable> --version` result, model, effort, ephemeral/session mode, effective-config schema/hash, absolute working directory, and exact fresh `codex exec` argv; `resume` is forbidden.
- Canonical autonomous role profile names are `supervisor`, `planner`, and `corrector`. `planner` and `corrector` exactly match the AW-01 worker-profile values; `supervisor` is an orchestrator role and is intentionally not a `WorkerProfile` value.
- The supervisor profile has decision-only, read-only authority. It may recommend exactly one structured next action from durable evidence but may not mutate repository/runtime evidence, commit, execute a worker, or take over Revolvr-owned safety, verification, transition, retry, or terminal-state policy.
- The planner profile has planning-only authority. It may return a structured new or revised durable plan grounded in supplied evidence, but it may not implement, verify, persist the plan, mutate task/runtime state, or route work.
- The corrector profile has finding/failure-scoped mutation authority. It may change only source/tests required by explicit verification failures or cited audit finding IDs and report the evidence; finding disposition, re-verification, re-audit, commits, retries, and overall completion remain Revolvr-owned.
- `internal/prompt.DefaultRunProfileTemplates` remains the canonical init seeding source, while `.agent/profiles/<name>.md` remains the runtime source loaded by `prompt.LoadRunProfile`. Init normalizes a template to one final newline only when creating a missing file and never reconciles or overwrites an existing repository-authored profile.
- Fresh supervisor execution lives in isolated `internal/supervisor`, not `runonce`, CLI, TUI, or worker lifecycle code. Its package-level API requires caller-supplied task identity, validated dossier, optional current audit, writable ledger, exact AW-05 invocation inputs, and injected command/snapshot dependencies; autonomous task discovery and selection remain deferred to AW-08.
- The AW-07 supervisor output schema is a deterministic standard-library projection of the single AW-01 `SupervisorDecision` contract. Codex receives it through a typed repository-contained `codexexec` output-schema path, while strict unknown-field decoding, an explicit EOF check, `SupervisorDecision.Validate`, exact task identity, and `ValidateCorrectionDecision` remain authoritative after execution.
- Supervisor raw output and canonical decision output are distinct artifacts: `supervisor-output.json` preserves exact output-last-message bytes for every produced response, while `supervisor-decision.json` is deterministic validated AW-01 JSON with a final newline and is materialized only after exact decoding and policy validation succeed.
- Supervisor source-read-only enforcement uses a source-writer lock plus validated content-sensitive before/after Git snapshots of HEAD, index, tracked content/modes/types, deleted paths, and non-ignored untracked content. Only `.revolvr/` runtime artifacts are excluded. Any mutation or capture uncertainty rejects the decision and is preserved as evidence; Revolvr never commits, resets, cleans, restores, or reverts the detected work.
- One supervisor invocation has its own ledger run. `completed` means exactly one decision was accepted, including a structurally valid `complete` or `block` recommendation; it never means the durable task completed. Rejected output, invocation failure, timeout, mutation, or capture uncertainty fails the supervisor run with verification `not_run`, empty commit SHA, no receipt, and no commit event.
- AW-07 returns a validated decision and a compact AW-02-compatible `DecisionReference` but does not persist it. Revolvr retains worker routing, verification, legal-transition, retry, commit, finding-disposition, completion-gate, and terminal-task authority for later slices.
- Canonical autonomous task lifecycle identity is `workflow: autonomous-v1`. Missing workflow remains a compatibility alias for `mixed-pass-v1`, and mixed-pass remains the default for task-file creation, app add, and Markdown import.
- The only AW-08 task-to-state reference is `autonomous_state_path: .revolvr/autonomous/tasks/<task-id>/state.json`. It is required for autonomous tasks, forbidden for mixed-pass tasks, must exactly identify the owning task namespace, and is validated without requiring or creating the referenced file. Existing symlink components are rejected so an apparently correct path cannot redirect into another namespace or outside the repository.
- Workflow lifecycle metadata is mutually exclusive. `mixed-pass-v1` keeps its ordered `implement|audit|document|simplify` phase and existing profile-frontmatter compatibility; `autonomous-v1` has no ordered phase and rejects a nonempty task profile because future worker selection belongs to validated supervisor routing. `internal/passpolicy` remains mixed-pass-only.
- Runnable task selection is workflow-explicit through `taskfile.ListRunnableForWorkflow` and `taskfile.SelectNextForWorkflow`. Compatibility `ListRunnable` and `SelectNext` mean the default mixed-pass workflow, and current `runonce` names that workflow directly. Every selector still validates all canonical task files and duplicate IDs before filtering, then orders eligible pending tasks by priority and filename.
- Status-only task updates preserve all non-status raw bytes when frontmatter already exists, including autonomous workflow/state references, unknown metadata, H1/specification content, CRLF/LF choice, and missing final newline. Metadata updates validate the complete resulting document before the existing atomic replacement; the APIs do not expose workflow or state-reference replacement, so retry cannot silently migrate a task or discard lifecycle evidence.
- AW-08 defines metadata, parsing, updating, read projection, and selection boundaries only. It does not create/load/persist AW-02 state, invoke the AW-07 supervisor, route workers, apply completion policy, add ledger evidence, execute verification, create receipts/commits, or add autonomous CLI/TUI execution behavior.
- Pure `autonomous-v1` routing policy lives in isolated `internal/autonomouspolicy`. Its sole public evaluation entry point is `Evaluate(Input) (Route, error)`, consuming AW-01 decisions, AW-02 decision references/state, and explicit transient evidence while returning only a compact worker/complete/block authorization. It does not belong in the ordered mixed-pass `internal/passpolicy` package.
- Autonomous policy source identity is an exact 64-character lower-case SHA-256 compatible with `gitstate.SourceSnapshot.SnapshotSHA256`, not HEAD or human-readable worktree status. Callers separately classify source safety as `safe`, `unsafe`, or `unknown`; all worker and completion routes require `safe`, while `block` remains available for unsafe or unknown classifications. An optional latest source mutation records same-task run/decision/action/resulting-revision provenance and nil means no source-mutating worker run exists in the supplied evidence history.
- Policy verification evidence reuses `autonomous.VerificationSummary` inside a source-revision envelope and requires nonempty run and occurrence identities plus typed evidence. It is fresh only when status is `passed` and its exact source revision equals the current revision; missing, failed, malformed, and stale verification fail closed wherever verification is a gate.
- Policy audit evidence embeds the exact validated `autonomous.AuditReport` and records audit run, worker profile, source revision, and consumed verification run/occurrence identities. A usable audit must come from `auditor`, cover the current source, consume the current fresh passed verification occurrence, and use a run distinct from the latest source-mutating worker run. Clean disposition is additionally required for document, simplify, and complete; correct requires `changes_required` and `ValidateCorrectionDecision`.
- Autonomous lifecycle admission is explicit: `pending` admits only plan and block; `ready` may consider every action subject to its gates; planning/working/verifying/auditing/correcting reject concurrent routing; needs_input rejects routing until AW-17 resume semantics; finalizing rejects routing until AW-20; completed/blocked/cancelled reject every new decision without an explicit reopen operation.
- Plan authorizes planner work without verification/audit and permits later revisions, but cannot bypass current unresolved blocking findings. Implement requires an actionable pending/in-progress step in a valid non-completed plan and cannot bypass any current unresolved changes-required finding. Audit requires current passed verification. Correct may select a deterministic unresolved subset of current changes-required findings and cannot select terminally resolved/waived/superseded/invalid findings. Document and simplify are optional independent routes after fresh clean gates and have no required order. Block requires the AW-01 typed decision evidence but no worker, verification, audit, or safe-source classification.
- Complete is authorization only and requires ready lifecycle, safe current source, a completed all-terminal plan, at least one nonpending structurally valid acceptance criterion, current passed verification, a linked fresh independent clean audit, and no open durable finding resolution. Existing AW-02 state-transition validation remains responsible for preventing durable plans, acceptance criteria, and finding resolutions from disappearing between snapshots; AW-09 evaluates the validated current snapshot and never constructs or persists a completed state.
- AW-09 policy performs no I/O, Git/Codex/verification commands, clock/random access, ledger/task/state persistence, task transition, receipt, commit, or runtime orchestration. It does not interpret attempt budgets, needs-input questions, optional-role outcomes, or retries, and it is not wired into supervisor, runonce, taskfile, app, CLI, or TUI behavior. `mixed-pass-v1` lookup/order and explicit runonce selection remain unchanged for later AW-10 integration.
- One autonomous supervisor-directed runtime cycle lives in isolated `internal/autonomouscycle`. Its only orchestration entry point is `Run(context.Context, Config) (Result, error)`; it composes AW-04, AW-07, AW-09, worker execution, receipt, verification, and commit evidence for one task without becoming an app, CLI, TUI, queue, daemon, or general mutable state runner.
- AW-10 receives one explicit validated `autonomous.ExecutionState` plus typed transient source-safety, latest-mutation, verification, and audit evidence. It deep-clones state for dependency calls and never loads, creates, migrates, updates, or returns a durable state snapshot. Canonical task loading is read-only and accepts only a pending exact `autonomous-v1` task with its AW-08 state reference; task and state metadata remain unchanged.
- `autonomouspolicy.ValidateEvidence` is the narrow structural pre-supervisor validation boundary for transient AW-09 evidence. It validates task/source/verification/audit envelopes without authorizing an action; `Evaluate` remains the sole route authorization and the cycle calls it exactly once after the supervisor and lock-held source admission.
- AW-10 brackets dossier assembly with validated full content-sensitive `gitstate.SourceSnapshot` values and refuses any same-HEAD, staged, tracked, deleted, untracked, mode, symlink, index, or HEAD drift without silently reassembling. The accepted supervisor's before/after snapshots must match that exact full dossier source baseline and remain read-only.
- The supervisor keeps its AW-07 internal source-writer lock. AW-10 never calls `supervisor.Run` while holding another source lock; after supervision it acquires a separate cycle/worker source-writer lock, rechecks the full source snapshot, evaluates policy under that lock, and holds it with heartbeat coverage through worker, change capture, verification, commit, and final capture.
- Full source snapshots and `CompareSourceSnapshots` remain the authoritative race/mutation record. The canonical AW-09/verification freshness identity is the pure `gitstate.PolicySourceRevision` content-tree SHA-256: it hashes non-missing path/type/mode/size/content entries but excludes HEAD, index placement, and missing tombstones. This keeps edits, additions, deletions, renames, modes, and symlinks content-sensitive while making staging and committing identical verified bytes administratively stable.
- A verification occurrence is always labeled with the exact pre-commit policy source revision it actually tested. After a successful commit, AW-10 captures the final full source and requires its policy revision to equal that verified revision; otherwise it returns a commit-freshness failure and never relabels the earlier verification as current.
- Supervisor and worker identities and artifacts are separate. The cycle supplies its own supervisor run/decision IDs to AW-07; only a validated worker route generates a distinct worker run ID. Worker prompt, provenance, output, source evidence, JSONL/stderr, receipt, ledger run, and artifact paths are isolated under the worker identity, and both Codex provenance records require fresh ephemeral `exec` argv with no `resume`.
- Worker profile selection comes only from the validated AW-09 route and exact repo-authored `.agent/profiles/<name>.md` content loaded through `prompt.LoadRunProfile`; autonomous task frontmatter cannot override it. The worker prompt embeds the exact dossier, decision, reference, route, profile, admitted source, artifact identities, and current evidence, forbids nested Codex/routing/commits/task-state mutation, and gives correctors an exclusive cited-finding authority projection.
- Plan and audit workers are source read-only. Any full snapshot mutation is failed evidence that Revolvr preserves without verification, commit, reset, clean, restore, checkout, or automatic correction. Implement, correct, document, and simplify may change source, but HEAD/task/state mutation, administrative-only Git changes, source-capture uncertainty, or changes outside exact observed path evidence stop before verification/commit.
- AW-10 treats receipt prose as advisory. It preserves exact worker output and original valid receipt claims, records mismatch warnings, and rewrites verdict, timestamp, changed files, verification, commit, and metrics from harness observations. Missing, malformed, and wrong-identity receipts use deterministic `receipt.FormatFallbackReceipt`; decision-only/read-only/no-change evidence remains in ledger/artifacts without forcing a source commit.
- Only a successful authorized source-changing worker with meaningful captured content changes receives one configured verification run and one harness-generated occurrence ID. Missing commands fail commit admission even when the existing verification package's configured missing-command policy reports pass. Failure, timeout, runner/capture/ledger error, or verification source mutation preserves evidence and starts no corrector, rerun, tier, retry, or second worker.
- AW-10 delegates commit staging, refusal classification, command evidence, pre/post-HEAD reconciliation, and ledger commit recording to `internal/commit`. It filters the post-worker capture to paths actually changed during this worker window so unrelated pre-existing work is not staged; refusal, failure, indeterminate HEAD, final capture uncertainty, and freshness mismatch remain explicit returned outcomes.
- Complete and block are transient AW-09 authorizations only in AW-10. They create no worker run/session, verification occurrence, receipt, commit, completed/blocked task status, terminal execution state, or `LatestDecision` update. Exactly one worker is the hard maximum for worker routes, and AW-10 contains no retry, automatic correction, re-audit, or repeated supervision loop.
- AW-10 has no CLI, TUI, app, `runonce`, task-selection loop, or mixed-pass integration. `runonce` continues to call `SelectNextForWorkflow(..., mixed-pass-v1)`, and `internal/passpolicy` remains exclusively the ordered mixed workflow; AW-11 and AW-12 own durable plan/acceptance and audit/finding persistence.
- AW-11 structured planner-output ownership lives in isolated `internal/autonomousplanning`. Schema `autonomous-planning-output-v1` contains the complete proposed current plan and acceptance matrix plus typed task, decision, worker, planner-profile, dossier, raw-output, and source provenance. Its standard-library decoder rejects unknown fields and requires exact EOF; receipt prose and arbitrary Markdown never become planning state.
- The canonical AW-08 state path remains `.revolvr/autonomous/tasks/<task-id>/state.json`, encoded directly as the current validated AW-02 `ExecutionState` with indented deterministic JSON and one final newline. AW-11 introduces no wrapper or migration and gives dossier/policy consumers the exact reopened state.
- Immutable planning transition history uses schema `autonomous-planning-transition-v1` under `.revolvr/autonomous/tasks/<task-id>/history/planning/`. A filename combines the zero-padded 20-digit resulting revision with SHA-256 of the operation ID, making ordering deterministic without exposing an unsafe operation string and making same-operation collisions explicit.
- State, history, planner output, dossier, profile, decision, task source, and source revision identities use exact byte sizes and lower-case SHA-256 values. Each history record retains exact previous/result state identities, plan identities and values, complete previous/result acceptance matrices, change kind/time, operation/application identities, and the full admitted planner provenance needed to reconstruct a prior revision.
- Planning persistence writes and synchronizes canonical validated planner output and immutable history before preparing and atomically replacing current state. A crash before replacement leaves the previous state authoritative and may leave only an identical orphan record; retry validates and reuses it. A committed state therefore always has required immutable evidence, while replay reopens, validates, and re-synchronizes the evidence/state directories after uncertain post-rename outcomes.
- `autonomousstate.Store` owns canonical loading and compare-and-swap persistence. It validates the canonical autonomous task and exact namespace, rejects existing symlink components and noncanonical/malformed/wrong-task state, serializes writers with a persistent per-task file lock, compares the caller's absent/existing plus hash/size expectation before history work and immediately before rename, uses same-directory temporary files with file flush, atomic rename, strict readback, and practical directory synchronization, and never silently changes missing/existing state expectations.
- Planning operation idempotency is content-addressed by an application SHA-256 over the exact expected state and admitted planner/cycle evidence. Replaying one operation ID with identical material returns the already committed state/history without another record; an identical orphan is reconciled; the same operation ID with materially different content returns an explicit conflict rather than overwriting evidence.
- An initial plan is accepted only from a policy-admissible pending state without a plan, at revision 1 with no predecessor, at least one ordered step, and no newly completed/skipped work. A deliberate ready-state revision requires a new plan ID, revision exactly plus one, and the exact current predecessor; stable step IDs cannot change meaning, terminal step status/evidence/rationale and compatible order cannot change, prior terminal work cannot disappear, and new terminal work is rejected.
- Acceptance criterion identity is stable by criterion ID and immutable exact requirement, with duplicate IDs and duplicate normalized requirements rejected. Existing criteria cannot disappear or be replaced under renamed IDs. A criterion source must be either the exact hashed canonical task artifact and text grounded in task bytes, or the exact current supervisor decision artifact and an exact cited supervisor success criterion; planner invention does not create task or supervisor authority.
- Acceptance transitions retain prior complete criterion values. Pending has no disposition evidence or rationale; satisfied requires validated typed evidence; waived and not-applicable require concrete rationale and retain any supplied validated evidence; nonpending criteria cannot return to pending. A revision may add a newly grounded criterion but cannot mutate or erase earlier acceptance evidence.
- A successfully applied initial plan moves `pending` to `ready`; a valid deliberate revision keeps `ready`. Both preserve task/schema identity, findings, attempts, budgets, and unrelated state, perform no retry accounting, and persist the exact current supervisor `DecisionReference` as `LatestDecision`. AW-02 `ValidateExecutionStateTransition` remains the final general transition authority.
- AW-10 remains a transient, generic one-cycle boundary. Only its plan/planner route receives the distinct planner JSON schema and raw-output artifact path; it still neither parses nor persists a plan. Isolated `autonomousplanapply.ApplyPlanningResult` later admits only a successful exact read-only AW-10 planner result with distinct valid supervisor/worker runs, matching decision/dossier/profile/source/output artifacts, fresh ephemeral invocation, no source mutation, and no synthesized verification or commit.
- AW-09 remains pure and performs no loading. A reopened AW-11 state is passed explicitly to `autonomouspolicy.Evaluate`; implement routing consumes the persisted plan, completion consumes the persisted matrix, and all existing verification/audit/finding/source freshness and independence gates remain unchanged.
- AW-11 does not persist audit reports, findings, or finding-resolution transitions; AW-12 owns those operations. It also adds no retries, corrections, verification tiers, needs-input semantics, terminal finalization, task-frontmatter updates, or generalized transaction database.
- AW-11 has no app, CLI, TUI, task-selection, or `runonce` integration. `internal/passpolicy` remains exclusively `mixed-pass-v1`, `Lookup(autonomous-v1, ...)` remains invalid there, and `internal/runonce` continues to explicitly select `mixed-pass-v1`.
- AW-12 structured auditor-output ownership lives in isolated `internal/autonomousaudit`. Schema `autonomous-audit-output-v1` contains one complete AW-01 `AuditReport` plus exact task, audit decision, auditor worker, auditor profile, dossier, source, verification occurrence, latest source mutation, and raw-output provenance. Strict standard-library decoding rejects unknown fields and requires exact EOF; receipt prose and arbitrary Markdown are never control evidence.
- Only the AW-10 `audit -> auditor` route receives `auditor-output-schema.json` and writes `auditor-output.raw.json`; the exact schema path is supplied to fresh ephemeral Codex. The cycle remains transient and read-only, performs no parsing/persistence, verification, or commit for audit, and leaves planner schema behavior unchanged while all other AW-12 worker actions remain schema-free.
- Canonical AW-02 state remains unwrapped deterministic JSON at `.revolvr/autonomous/tasks/<task-id>/state.json`. AW-12 adds no mutable current-audit projection. Exact current audit authority is reconstructed from immutable history only when a transition's resulting state hash and size match canonical state.
- Immutable audit and finding-transition history uses schema `autonomous-audit-transition-v1` under `.revolvr/autonomous/tasks/<task-id>/history/audit/`. Filenames combine a zero-padded 20-digit global audit-transition sequence, the typed transition kind, and SHA-256 of the operation ID. Audit revisions increment only for recorded audits; finding transitions retain the introducing/current audit revision and exact report/policy projection.
- Every audit history record retains the exact audit decision/reference, supervisor artifact, auditor run/profile, dossier identity, audited source revision, passed verification run/occurrence, latest source-mutation identity, task/raw/canonical artifacts, report, policy evidence, previous/resulting state identities, complete previous/resulting resolution slices, and any finding transition evidence/rationale/authority/target.
- Audit and planning persistence share the exact existing per-task `state.lock`. Audit persistence performs canonical task/path/symlink validation and exact hash/size CAS before immutable work and immediately before state rename. Canonical audit output and history are synchronously created before same-directory temporary state replacement; atomic rename, directory sync, and strict readback follow.
- A pre-state crash may leave orphan canonical/history files, but they never become current. Committed-history reconstruction walks backward from canonical state across both audit and planning transition identities, excluding orphans from durable finding identity. Identical retry reuses an orphan's sequence/revision; a post-rename state always has reconstructible immutable evidence.
- Audit operation idempotency is content-addressed over exact expected state plus admitted cycle/raw/canonical evidence. Exact replay returns the existing state/history, while one operation ID with different material returns `ErrOperationConflict`. Stale state produces `ErrStaleWrite`; typed application results retain concrete failure stage and reason.
- Audit admission requires the exact successful AW-10 worker route, task/action/auditor profile, supervisor decision/reference, completed worker ledger run, fresh ephemeral schema invocation, readable matching task/dossier/profile/schema/raw artifacts, unchanged source/task/state, no synthesized verification or commit, and exact current passed verification/source evidence. Supervisor, auditor, verification, and latest source-mutating run identities must be distinct where applicable; these independence relationships are revalidated from persisted history.
- Finding IDs are durable identities. A reused ID keeps significance and deterministic normalized summary/correction meaning; prior evidence is an exact retained prefix and only deterministic appends are allowed. An equivalent new ID is a probable rename while the old finding is open; open findings cannot disappear; terminal IDs cannot reopen or be reused; terminal recurrence needs a new identity.
- Clean audit behavior is fail-closed when any finding remains open: silence never resolves, waives, supersedes, invalidates, deletes, or renames durable history. Clean reports may coexist with historical terminal resolutions, which remain visible in state and dossiers.
- Finding resolution is an explicit separate AW-12 application. Resolved, waived, superseded, and invalid all require typed evidence; waived/invalid require rationale; supplied resolution verification must be passed/current and retained exactly; correction decisions must cite the finding; supersession requires a different known noncontradictory target and is acyclic. No reopen semantics are added.
- Successful audit persistence keeps lifecycle `ready`, preserves plans, acceptance, attempts, budgets, and unrelated state, appends open entries only for newly admitted findings, and stores the exact audit decision as `LatestDecision`. Resolution normally remains `ready` and updates `LatestDecision` only when an explicit supplied authority reference exists.
- Reopen strictly validates every persisted JSON envelope and required artifact identity, reparses raw and canonical auditor output, requires canonical bytes, and rejects raw/canonical/history disagreement. Reopened `autonomouspolicy.AuditEvidence` is passed explicitly to pure AW-09; reopened reports and state are passed explicitly to input-driven AW-03/AW-04 dossier assembly.
- AW-12 provides persistence APIs only. It does not execute a corrector, verification, automatic re-audit, or repeated supervisor loop; AW-14 owns that orchestration. It adds no AW-13 verification tiers/flaky reruns and no app, CLI, TUI, task-frontmatter, `passpolicy`, or `runonce` integration.
- AW-13 tier-plan ownership lives in isolated `internal/autonomousverification`; `internal/verification` remains the low-level bounded flat command runner. The autonomous package owns strict versioned plan/result/gate contracts, pure selection, exact identities, one-operation orchestration, controlled flaky classification, deterministic artifact encoding, and legacy projections, but owns no task state, audit persistence, correction, commit, or lifecycle transition.
- Canonical tier-kind order is `structural`, `focused`, `task_acceptance`, `full_suite`, `race`, `integration`, then `security`. Plans require unique lower-case kebab-case tier IDs, unique kinds, canonical order, declared command order, typed flags/policies, repository-contained command directories, and no public map contract. Caller slices are copied before selection or execution.
- Verification purpose is explicit: `fast` selects only explicitly enabled structural/focused/task-acceptance tiers and is evidence only for those checks; `final` selects every explicit final tier and requires at least one configured required final tier. Optional final tiers run only when enabled. A final gate requires an ordinary passed overall operation and ordinary passed evidence for every configured required tier.
- Existing flat command configuration maps deterministically to one `legacy-flat` tier of kind `full_suite`, required and enabled for final verification with reruns disabled. Omitted tier config continues to execute the exact existing flat runner. Supplying flat and tiered material together is an error. Empty legacy command YAML retains existing defaults rather than becoming an ambiguous tier plan.
- Plan identity is SHA-256 plus byte size over validated indented canonical plan JSON with one final newline. Effective configuration hashing additionally covers tier IDs/kinds/order, fast/final/required flags, rerun policy, exact ordered command fields, and global default timeout/caps. Command material identity separately binds plan hash, purpose, tier, position, executable, argv, normalized directory, ordered env, and effective timeout/caps.
- Attempt identity is caller-injected and distinct from the verification occurrence. Attempt evidence is immutable and append-only: attempt number, command identity, times/duration, exit and classification, timeout/cancellation/runner error, capped stdout/stderr, and truncation counts remain visible. A second attempt never overwrites the first.
- Tiered execution is fail-fast. Commands run in declared order, tiers run in canonical selected order, and the first nonpassing command/tier prevents later commands/tiers. Tier start/completion evidence is retained for everything reached, and cancellation starts nothing further.
- A flaky-classification rerun is permitted only for the first deterministic ordinary command failure whose tier explicitly uses `once_to_classify_flaky`; the entire operation has a one-rerun maximum. Missing commands, timeout, cancellation/context cancellation, runner errors, invalid configuration, ledger/artifact failures, and already-rerun work are ineligible. No third or recursive attempt exists.
- Fail/fail is failed and retains both attempts. Fail/pass is `flaky` and retains both attempts. Flaky is intentionally not passed because a nondeterministic required check is not trustworthy final evidence; it fails aggregate compatibility projection, final-gate satisfaction, receipt overall status, and commit admission.
- Missing selected commands, ordinary failures, timeout, cancellation, runner errors, output truncation, ledger failure, artifact failure, invalid configuration, and verification source mutation remain explicit rather than becoming a generic blocked verification result. Truncation is evidence and does not alone turn an otherwise successful command into failure.
- Final-gate validation recomputes authority from the plan identity, purpose, required/selected/executed tier slices, required outcomes, missing required tiers, and overall outcome. Consumers do not trust a standalone `final_satisfied` boolean. Legacy evidence without a tier projection remains compatible through the documented adapter.
- AW-10 purpose selection is `final` for implement and source-changing document/simplify, and explicitly configured `fast` for correct. Plan/audit read-only workers, failed workers, and no-change workers run no verification. One policy rerun remains part of one verification operation and never starts another worker or cycle.
- AW-10 commits only after an ordinary pass for the selected purpose and unchanged post-verification source evidence. Fast correction evidence may admit that correction's commit but never claims the final gate; flaky, failed, missing, timeout, cancelled, errored, artifact/ledger-failed, or source-mutating verification never commits.
- AW-09 remains pure. `autonomouspolicy.VerificationEvidence` consumes a validated gate projection and the compact summary can retain the exact typed result. Audit/document/simplify/complete require current final-gate evidence when tiered; fast-only, unsatisfied, malformed, stale, or wrong-identity evidence rejects. Tier selection and flake classification never occur inside `Evaluate`.
- AW-12 auditor provenance retains exact tier gate/result material alongside existing run/occurrence/source identity. Strict raw/canonical/history parsing and reopen validation reject fast-only, flaky, failed, malformed, or provenance-mismatched tier evidence without changing audit/finding storage layout or identity rules.
- Tiered verification artifacts live at `.revolvr/runs/<worker-run-id>/verification.json`. Ledger events retain plan/purpose/selection, tier order, command identities, attempts, rerun authorization, classification, final gate, and artifact identity; artifact extraction exposes the path after reopen. Dossiers render a bounded typed projection including both flaky attempts and explicit failure facts. `revolvr.receipt.v1` stays unchanged and represents attempts as ordered verification entries; distinct outcomes for the same command are not deduplicated.
- AW-13 adds no automatic correction, final re-verification, re-audit, repeated supervision, retry budget, or task-state transition; AW-14 owns those loops. It adds no CLI/TUI autonomous execution and no `runonce` autonomous path. `internal/passpolicy` remains exclusively `mixed-pass-v1`, while `runonce` explicitly refuses tiered autonomous plans instead of changing mixed-pass behavior.
- AW-14 bounded correction orchestration lives in isolated `internal/autonomouscorrection`. One call may run exactly one correction cycle, one distinct final verification, explicit resolutions for only corrector-claimed cited findings, one fresh audit cycle, and one audit application; it contains no retry or recursive supervision loop.
- Correction authority is an exact union. Existing audit corrections retain `finding_ids`; verification-triggered correction uses `autonomous.VerificationFailureTarget` with exact task, run, occurrence, source revision, failed status, and typed evidence. A correction decision cannot combine or substitute those authority kinds, and pure AW-09 policy compares verification authority to the exact supplied failed occurrence.
- Correctors now return strict schema `autonomous-correction-output-v1`. The result binds task/worker/decision/authority, partitions cited findings into resolved and remaining IDs, and carries typed evidence. It cannot claim an uncited finding, and a verification repair cannot invent an audit finding. Failed and partial outcomes remain explicit control evidence.
- Corrector execution remains one ordinary AW-10 source-changing cycle with fast verification and commit admission. Correct receives a corrector-only schema/artifact, but `autonomouscycle.Run` still performs exactly one supervisor decision and at most one worker; it does not run final verification, resolve findings, or re-audit itself.
- The AW-14 coordinator captures and returns an exact pre-correction full source checkpoint and rejects dirty or ambiguous source before another writer. It never resets, cleans, checks out, restores, rolls back, or destructively rewrites user work.
- Final verification has its own ledger run and occurrence, targets the committed corrected source, starts after correction completion, and must be a validated ordinary final-purpose pass satisfying every configured required tier. Any other outcome or source mutation stops before audit while retaining the attempt.
- Finding resolution remains exclusively through `autonomousauditapply.ApplyFindingResolution`. Resolution evidence combines exact structured correction evidence/artifact, the exact correction decision/reference, and the new final verification; partial repair leaves other findings open.
- Re-audit is a separate fresh AW-10 auditor cycle on the corrected source and final verification occurrence. Its supervisor/auditor identities must differ from correction supervisor/corrector/final-verification identities, and it starts after final verification. Persistence remains through `autonomousauditapply.ApplyAuditResult` and immutable `autonomousstate` history. Clean or findings-bearing persisted re-audits both return control to supervision and never directly complete the task.
- AW-14 adds no app, CLI, TUI, task-frontmatter, `runonce`, or `passpolicy` path. `autonomouspolicy.Evaluate` remains pure, and AW-15 retains all retry-budget, no-progress, repeated-signature, strategy-change, elapsed/token-limit, and circuit-breaker behavior.
- AW-15 ownership model: a new isolated `internal/autonomousattempt` boundary authoritatively admits and completes exactly one caller-selected action above the existing one-shot cycle/correction APIs. Admission is compare-and-swap persisted before the injected runner starts and charges the total and exact action attempt once; completion uses the admitted resulting state as its compare-and-swap expectation and applies trusted harness duration and receipt token facts once. The supervisor supplies no budget fields and cannot write accounting state.
- AW-15 limits are explicit caller authority. The first clean attempt may bind caller-supplied unset/limited/unlimited total, per-action, elapsed, and token limits into state; every later call must supply the exact same modes and limits. Limited budgets admit only while `consumed < limit`; equality is exhausted for attempt counts, elapsed time, and tokens. Consumption is monotonic, and missing token facts under a limited token budget are a typed fail-closed stop rather than guessed usage.
- AW-15 durable evidence uses append-only typed attempt events in canonical state plus one immutable compare-and-swap history record per admission/completion/breaker operation under the task namespace. Exact task/action/attempt/decision/run/occurrence/source identities, outcome, trusted duration/token facts, evidence references, canonical decision/failure/finding signatures, and canonical structured strategy signature remain restart/replay inputs. Operation IDs are idempotent and content-addressed; stale writers cannot double-charge.
- AW-15 progress detection is exact and deterministic. Signature helpers hash validated structured decisions, exact verification-failure occurrences, and stable ordered open-finding/report identities. Strategy signatures hash normalized typed approach/technique/target material while excluding run IDs, timestamps, formatting-only whitespace, and incidental explanatory prose. A caller-owned repeated-signature threshold is persisted with the limits; a transient first failure may retry, but threshold repetition terminates, and an explicit no-progress event requires a materially different strategy before another admission.
- AW-15 task isolation follows the existing canonical task namespace and shared per-task `state.lock`. A circuit breaker appends typed reason, budget snapshot, triggering attempt/signature references, and immutable history evidence, then transitions only that task through existing state validation to `blocked`. It cannot complete a task, resolve findings, erase evidence, or start a later stage.
- AW-16 optional-role ownership lives in isolated `internal/autonomousoptional`. A trusted, versioned `OptionalRoleAssessment` binds one documentor or simplifier decision to exact task/state/source/final-verification/audit evidence. Documentation obligations and simplification targets are structured harness evidence with exact repository-relative target paths; supervisor prose cannot invent or waive them, and every observed changed path must remain within selected target authority.
- Documentation and simplification remain independent and unordered. `not_applicable` is a typed decision-only occurrence that requires exact `no_relevant_work` evidence and rationale and consumes no AW-15 attempt. It is not absence of an attempt, task-frontmatter state, an acceptance disposition, or a receipt claim. Existing completion still requires neither optional role by default.
- Executed optional roles use one AW-15 admission/completion around one ordinary AW-10 role cycle and, only after a committed source change, at most one fresh independent audit cycle plus the existing AW-12 audit application. The nested audit is a stage of the one charged operation. No-op remains AW-15 `no_progress`, runs no verification/audit, creates no commit, and retains exact worker/receipt/ledger/artifact/current-gate evidence.
- Source-changing optional roles rely on AW-10 final-purpose verification and exact commit admission. The coordinator requires a newer independent audit for the same source and verification occurrence before success; simplification additionally retains behavior-preservation evidence. It never resolves or otherwise dispositions audit findings, and both clean and findings-bearing audits return to supervision.
- Optional-role occurrences are append-only in canonical state and immutable schema `autonomous-optional-role-transition-v1` history under each task namespace. Persistence shares `state.lock`, exact CAS, history-before-state ordering, strict readback, and content-addressed replay. Audit reconstruction traverses planning, audit, attempt, and optional-role edges so evidence-only transitions do not hide the latest audit; policy still rejects stale source or verification identities.
- Optional-role relevance identity canonicalizes exact evidence and selected IDs while excluding supervisor IDs, timestamps, formatting, and rationale-only changes. The full occurrence separately retains the exact validated decision/reference and rationale, so materially conflicting operation reuse still fails closed.
- AW-17 models operator input as a distinct terminal-for-now supervisor action, `needs_input`, rather than block rationale. The action selects no worker and forbids strategy, correction authority, and success criteria. Its typed question contains exact task/question/revision/content identity, question and blocking reason, stable mutually exclusive options, one offered recommendation and rationale, typed evidence, and optional independent-work declarations.
- Needs-input content authority is SHA-256 over compact deterministic JSON containing every control-relevant question field except the hash itself. A supervisor may omit the hash so the harness assigns it deterministically during strict parsing; a supplied hash must match. Answers and resumes use only exact typed task/question/revision/content/option/answer identities, never labels, array indexes, recommendation prose, or rationale as control authority.
- Independent work under needs-input is a projection, not a route. It is valid only for compatible read-only plan/planner or audit/auditor declarations whose ordered `independent_of_option_ids` exactly names every offered option and whose inputs are typed evidence. AW-17 never executes such work; later scheduling may consider it only after the pure clean-yield gate succeeds.
- Canonical execution state keeps append-only input lifecycle evidence as ordered question, answer, and resume records while `NeedsInputDetail` holds only the exact current question identity. A question records source revision, decision provenance, suspension time, resume lifecycle, and explicit predecessor identity. An answer records stable answer ID, one offered option, operator provenance/evidence, and harness time. Resume records bind that exact answer and restores only the question's recorded pending/ready lifecycle. Questions/answers/resumes never consume or reset AW-15 accounting.
- Input persistence lives in isolated `internal/autonomousinput` over `internal/autonomousstate` schema `autonomous-input-transition-v1`. It uses the canonical task namespace and shared `state.lock`, exact state hash/size CAS, immutable history before atomic canonical-state replacement, synchronized files/directories, strict readback, content-addressed replay/conflict behavior, and before/after-rename recovery. Committed audit reconstruction traverses input edges alongside planning, audit, attempt, and optional-role edges.
- `autonomouspolicy.Evaluate` remains pure and returns a distinct no-worker needs-input route only from pending/ready safe state. Suspended state admits no ordinary route before exact answer plus explicit resume. The separate pure future-scheduler projection returns typed clean/unsafe results for task/state/question/provenance/source/in-flight gates; dirty, unknown, stale, malformed, legacy, or ambiguous state never yields. No scheduler, CLI, or TUI control surface was added.
- Legacy minimal `needs_input: {reason: ...}` execution state remains readable and is labeled non-answerable in dossiers. It cannot fabricate question IDs, revisions, options, recommendations, content identities, answers, or resume authority, so input application fails closed until a typed question is durably recorded.
- AW-18 uses `autonomous.TaskWorkspace` as the typed control/execution-root authority. It records canonical absolute control root, execution worktree, Git common directory, control-root ownership marker, deterministic harness branch, exact baseline/current/checkpoint identities, retained refs, and recovery status; immutable ownership cannot change and checkpoint/retained evidence cannot regress.
- Workspace IDs hash canonical control root, Git common directory, and task ID. Harness worktrees live only under `.revolvr/autonomous/worktrees/<workspace-id>` and refs under `refs/heads/revolvr/tasks/`; a canonical synchronized control-root marker is required before any existing ref/path/registration may be recovered. Naming conventions or path presence never prove ownership.
- Workspace preparation uses an exact commit object, never primary index/filesystem bytes. Marker-before-ref and ref-before-registration states are recoverable; foreign branches, worktrees, paths, symlinks, markers, common directories, or `.git` links are conflicts. Unknown advanced HEADs are retained under immutable `refs/revolvr/retained/` inspection refs rather than reset or guessed current.
- The known-good checkpoint begins at the baseline and advances only from an exact reconciled clean commit/tree/source identity with operation provenance. Restoration first retains failed staged/tracked/untracked bytes in an immutable commit/ref, then may reset/clean only the twice-revalidated owned execution worktree to the exact durable checkpoint. Ignored files that cannot be safely retained block restoration.
- Workspace transitions use `autonomous-workspace-transition-v1` immutable history and the existing per-task `state.lock`, exact state CAS, history-before-state atomic replacement, strict readback, replay/conflict/stale classification, and committed-audit graph traversal. A control-root global Git-admin flock serializes ref/worktree mutation; workspace source locks remain control-root state while naming the exact protected execution root.
- Autonomous source execution is fail-closed and root-separated. `autonomouscycle.Run` requires a validated workspace matching durable state. Canonical tasks, profiles/guidance, state/history, ledger, receipts, and artifacts remain at the control root; source snapshots, Codex work, changed-file capture, verification, commits, correction, and optional-role mutations use the execution root. Codex and tiered verification carry a separate control-root artifact root. Mixed-pass behavior remains unchanged.
- AW-19 autonomy declarations use schema `revolvr-autonomous-safety-declaration-v1` with explicit `operator_attended` and `fully_unattended` modes. Operator-attended is the compatibility default for mixed-pass and current dogfood; it renders dangerous bypass, ambient environment, operator-attended hooks, and unknown network posture as operator responsibilities. Fully unattended is never inferred or defaulted and requires an exact task/workspace-bound preflight plus policy acknowledgement.
- Effective run configuration is now `revolvr-effective-run-config-v2`; its deterministic projection includes the complete secret-name-only autonomy declaration. Secret values never enter effective configuration or its hash. The policy acknowledgement is `revolvr-fully-unattended-v1:<policy-sha256>` and authorizes the nonrecursive material policy identity; changing workspace, permissions, commands, roots, external isolation, network, hooks, environment, or redaction policy invalidates it.
- `internal/autonomoussafety` owns pure typed safety contracts and bounded host preflight. It consumes the exact ready/restored AW-18 workspace and current source identity, derives harness/model writable roots and protected path classes, resolves executable hashes/argv/directories/environment/timeouts/caps, inspects effective `core.hooksPath` without running or modifying hooks, distinguishes unknown/denied/restricted/unrestricted network posture, requires stable external attestations for full autonomy, and states that worktrees are Git/source isolation rather than a security sandbox.
- Autonomous safety preflight runs inside `autonomouscycle` after canonical task/workspace/source/config authority is known and before dossier assembly or supervisor Codex. Unsafe or mismatched authority returns `safety_preflight_failed` and starts no supervisor, worker, verification, or commit. When AW-15 has already durably admitted the enclosing operation, the safety stop remains charged once and is classified as trusted safety-stop evidence; no duplicate admission occurs.
- The same safety-policy SHA-256 and complete redacted policy/preflight projection are recorded in supervisor and worker provenance and ledger preparation evidence. Model-authored changed paths are checked against protected Git administration, task specs, profiles, guidance, state/history, ownership, locks, ledger, and config authority before verification or commit; task/model output never supplies commands, environment, writable roots, hooks, network policy, or bypass authority.
- `internal/redact` provides dependency-free deterministic longest-first configured-secret replacement using explicit environment-variable names. Values are loaded only at runtime, short/missing/duplicate/malformed sources fail closed, and persistent Codex JSONL/stderr/final-output, returned output/errors, progress, and ledger summaries receive the redacted form plus source/match facts. Autonomous command results are redacted before verification/Git/commit evidence, and fully unattended execution can replace rather than inherit the ambient process environment through the bounded runner.
- AW-20 terminal ownership lives in isolated `internal/autonomousfinalization`; it consumes one already-authorized complete decision and performs no Codex, worker, verification, audit, correction, optional-role, commit, restore, archive, or scheduling work. Pure completion authority is recomputed through `autonomouspolicy.Evaluate`, and a caller-supplied bounded live-evidence revalidator must confirm the frozen workspace/source/safety authority before admission and before task terminalization.
- Canonical state uses `autonomous-finalization-state-v1` with monotonic `admitted`, `capsule_materialized`, `task_completed`, `state_completed`, and `ledger_completed` stages. Immutable operation, run, frozen-evidence, original/completed-task, capsule, manifest, and harness-time identities cannot be rewritten; legacy completed snapshots without AW-20 detail remain readable, while a real `finalizing` snapshot requires it.
- Frozen authority is map-free schema `autonomous-completion-frozen-evidence-v1`. It binds exact canonical task/spec and state bytes, complete decision/reference/route, source and final-purpose tier gate, independent clean audit, plan/acceptance/findings/optional-role/attempt evidence, exact ready/restored checkpointed workspace, safety policy/preflight/config, ordered commit/run evidence, provenance, and harness times. Full state/task hashes are recomputed, no attempt may remain in flight, and final HEAD must equal the checkpoint and final commit evidence.
- Completion artifacts live only under `.revolvr/autonomous/tasks/<task-id>/completion/` as `completion-evidence.json`, deterministic `completion.md`, and `completion-manifest.json`. Writes are immutable, same-directory temporary-file/file-sync/rename/directory-sync operations with containment, symlink-parent refusal, exact readback/hash/size checks, collision refusal, and configured-secret redaction. Schema `autonomous-completion-capsule-manifest-v1` records ordered source identities and explicit omissions without recursive self-hashing.
- AW-20 changes the canonical autonomous task status from `pending` to `completed`; AW-21 still owns tracked archive movement and reopen. The expected completed task projection is frozen and validated before admission, so a crash after the task rename cannot authorize changed task bytes. No metadata-only source commit is created.
- Transaction order is frozen artifact, admitted history/state, exact finalization run/prepared evidence, capsule and manifest, materialized history/state, exact task-status transition, task-completed history/state, completed lifecycle/terminal history/state, exact terminal ledger event/run completion, then ledger-completed history/state. Every state replacement uses `autonomous-finalization-transition-v1` under the shared per-task `state.lock`, history-before-state CAS, atomic replacement, strict readback, replay/conflict/stale classification, and committed-audit graph traversal. Once durable progress begins, reconciliation uses a non-cancelled context.
- Finalization ledger events are versioned exact payloads for prepared, capsule-materialized, state-terminal, and terminal-completed effects. Retry reads and compares the deterministic run/event/completion evidence before writing; identical effects replay, different effects conflict, and terminal ledger completion can never precede the capsule/manifest it cites.
- AW-21 archive ownership lives in isolated `internal/autonomousarchive`. Its bounded create/list/show/verify/reopen operations do not invoke Codex, verification commands, audit, correction, optional roles, workspace restoration/cleanup, a scheduler, or a repeated task loop. Active task discovery remains exclusively `.agent/tasks/*.md`; archive discovery is an independent strict scan of `.agent/archive/YYYY/MM/<task-id>/archive.json`.
- Archive disposition authority is schema `autonomous-task-archive-authority-v1` and covers exactly `completed`, `cancelled`, `superseded`, and `abandoned`. Task status and autonomous lifecycle contracts represent those four terminal outcomes; blocked and needs-input remain active and are rejected. Non-completed admission requires matching terminal task/state disposition and reason plus explicit trusted provenance/time, and explicitly omits AW-20 completion authority.
- Completed archive admission requires AW-20 `ledger_completed` state, exact completed task bytes, canonical terminal state, frozen evidence, completion manifest/capsule, source/workspace/checkpoint, final verification, independent clean audit, safety policy, and exact terminal finalization ledger event/run identities. Archived `completion.md` is a byte-identical copy of the verified AW-20 capsule; AW-21 never regenerates it.
- Archive IDs are deterministic SHA-256-derived identities over task, operation, disposition, and one explicit UTC archive time. That time alone selects `.agent/archive/YYYY/MM/<task-id>/`; task/capsule/manifest paths and archive/task IDs are collision-fail-closed, and strict archive scanning rejects duplicates, foreign files/directories, traversal, symlinks, unsafe permissions, and hard-link surprises.
- Tracked schema `autonomous-task-archive-manifest-v1` binds task/disposition/reason/provenance/terminal/archive times, original and archived task identity, canonical state, applicable AW-20 artifacts/finalization/ledger identity, expected tracked paths, and explicit omissions. It intentionally does not self-reference the administrative commit; the reconciled SHA lives in immutable runtime history and exact archive ledger evidence.
- Archive transactions take the control-root Git-admin lock before the per-task `state.lock`, reject live control/workspace source writers, and validate active task/state/artifacts/ledger/Git/index/worktree/operation authority before admission. Each monotonic stage writes synchronized immutable `autonomous-task-archive-transition-v1` history before atomic mutable journal replacement, then publishes task/capsule/manifest, removes only the exact active task, reconciles the administrative commit, and completes exact ledger evidence. After admission, roll-forward uses a non-cancelled context; exact retry reuses effects and conflicting partial evidence remains inspectable.
- Administrative archive commits are path-scoped and carry `Archive-Operation`, `Archive-ID`, `Task-ID`, `Disposition`, and `Terminal-Identity` lines. Pre/post HEAD reconciliation supports unborn repositories, retries HEAD lookup, accepts an advanced verified HEAD despite command failure, and rejects indeterminate outcomes. Commit verification checks the exact addition/deletion set and exact archive bytes; unrelated staged, dirty, untracked, conflicted, or source-worktree paths are never absorbed.
- Archive verification is read-only and returns ordered named checks. It uses immutable SQLite access and read-only Git object commands to cross-check archive path/date/schema, task bytes/metadata, terminal state, AW-20 evidence/capsule/manifest, finalization ledger evidence, immutable archive history, archive ledger run/events, administrative commit message/tree/bytes, active-task exclusion/reopen lineage, and configured-secret absence without repair or sidecar creation.
- Reopen uses a new task ID and schema `autonomous-reopen-lineage-v1`; terminal archives and old runtime history remain immutable. A verified archive produces one new pending autonomous task/state with only id/status/state-reference metadata rewritten and exact spec/unknown metadata/line endings preserved. Lineage binds archive/task/disposition/commit, operator authority/reason, operation, and UTC time. The new task addition receives its own path-scoped administrative commit, exact replay recovers partial publication, conflicting or second reopen attempts fail, and no task execution starts.
- AW-23 scheduling metadata uses source-ordered comma-separated scalar lists: `depends_on`, `tags`, and `conflicts`. Items are stable nonblank safe identities without comma or surrounding whitespace; duplicates and self-dependencies are invalid. Source order is identity-significant and is preserved rather than silently sorted. Child tasks additionally carry exact `parent_task_id`, proposal/decision/run IDs, evidence tokens, and `parent_behavior`; status-only updates and archive/reopen projections do not rewrite those bytes.
- `internal/autonomousscheduler` is the pure graph/readiness owner. Repository loading parses and duplicate-checks every active task before filtering, and exact archive evidence is a separate typed input. Missing dependencies, duplicate identities, active/archive coexistence, unverified archive authority, and deterministic directed cycles fail closed. Completed active tasks and verified/reconciled completed AW-21 archives satisfy edges; every other lifecycle/disposition waits, and archives are never runnable nodes.
- Ready order preserves the established lower-integer-first priority direction, then canonical source path, then task ID. A conflict applies against exact occupied identities when either task names the other or both share a stable key. AW-23 remains sequential and creates no parallel-worker or fairness policy.
- New AW-22 operations apply scheduler admission before workspace/model work. Omitted task IDs select once; explicit IDs must classify ready. A previously admitted operation's durable pinned task remains stronger than changed ambient graph/priority/archive state, so restart never reselects and one operation never advances to a second task.
- Supervisor child proposals remain part of the existing no-worker `block` or `needs_input` decision rather than adding a queue action. The strict schema permits at most four children and bounds canonical proposal bytes, titles, scopes, criteria, evidence, and graph lists. Material scope evidence must exactly occur in accepted decision inputs. Dependent children name their parent edge; independent children cannot name, answer, or bypass the parent. Publication consumes no AW-15 worker attempt.
- `internal/autonomouschild` owns deterministic ID derivation, exact child task/state projection, graph-impact validation, and restartable publication. Initial state is pending with immutable `autonomous-child-lineage-v1`; ordinary state transitions cannot attach, remove, or rewrite that lineage. Parent bytes/state/history are hash-revalidated and never written. Active task specs remain ordinary uncommitted canonical task files in this slice; AW-23 does not infer Git administrative commit authority.
- Child publication takes `.revolvr/locks/child-publication.lock` without holding the AW-22 operation lease, Git-admin lock, a parent/child state lock, or a workspace source-writer lock. It writes immutable history before each mutable journal stage, publishes every child state before task bytes, refuses collisions, and makes incomplete publication authority a scheduler error until exact roll-forward. Three content-compared supervisor-run ledger events are replay-safe. Configured secret values are rejected before persistent child task/state evidence.
- App status and task projections carry dependency, tag, conflict, parent, readiness, and deterministic next-autonomous facts. CLI task list exposes narrow scheduling columns, status names next autonomous readiness, and TUI `U` uses the same readiness result. `runonce` and `passpolicy` remain exclusively mixed-pass and do not consult the autonomous graph.
