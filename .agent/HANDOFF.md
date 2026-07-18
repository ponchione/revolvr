# Active Work Handoff

Updated: 2026-07-18

## Resume Point

The first unchecked backlog task remains `EXT-20`, the quantitative Level-1
real-Codex dogfood gate. Do not restart RC.1 or RC.2 work. RC.3 local
reproducibility, exact-source remote CI, remote artifact attestation, the
guarded suite update, and a fresh collision-free no-model preparation have
passed. No live model operation has started.

Prepared live authority is `/tmp/revolvr-ext20-rc3.Qghf19/suite`. The
controller independently verified it and committed/pushed the tracked suite,
state, and handoff changes with raw Git. A new explicit live confirmation is
now required before the recorded command may run. Do not use the
failed-inspection diagnostic root
`/tmp/revolvr-ext20-rc3.5TQPha/suite`.

## Git And Release Authority

- Reviewed workflow commit: `80441464d55af466bbea15f20448099e2a163684`
  (`Add RC.3 artifact attestation workflow`). The remote-attestation evidence,
  suite-preparation evidence, and current handoff are committed on top of it;
  use `git rev-parse HEAD` for the exact resumed `main` tip.
- RC.3 candidate source: `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`.
- RC.3 source tree: `23c0d27fc62be5f41feb45192e74f1df8ecff3fa`.
- Remote candidate ref: `refs/heads/level1-v0.1.0-rc.3` at the exact source
  commit above.
- Candidate CI: run `29642126354`, exact source SHA, conclusion `success`, all
  ten mandatory jobs passed.
- RC.3 attestation ref `refs/heads/level1-v0.1.0-rc.3-attestation` resolves to
  exact reviewed workflow commit `80441464d55af466bbea15f20448099e2a163684`.
- Dedicated attestation run `29651665454` and job `88098916882` completed
  `success`; general CI run `29651665483` also completed `success` with all ten
  required jobs passing.
- Use raw `git` only. Never use `gh`. The standing operator direction is to
  commit and push confirmed passing work with raw Git.

## RC.3 Artifact Authority

- Candidate ID/version: `level1-v0.1.0-rc.3`, release `0.1.0`.
- Toolchain: exact Go 1.26.5 for release builds; Go 1.22.12 source floor.
- Linux amd64 SHA-256:
  `9e9c13f43977edf49e7e6385c595aa20a01c16308ddfea1c30455ea88252ae9b`.
- Darwin amd64 SHA-256:
  `ee6db29cfcbcbd2e645184927fc5d7348ed924d6036a46d7b23eae55b5b43fd4`.
- FreeBSD amd64 SHA-256:
  `6a42dc423ab1975e8ea4296f56be6c9a3773d9ccfda1c57244a50261e64f368a`.
- Candidate bundle:
  `.revolvr/release-candidates/level1-v0.1.0-rc.3-a16ea1bdc1a4/`.
- Candidate inventory SHA-256:
  `766856c2783073c8ffa10cbce0e3c0a9f8ebee4db1785c309f6c5680f5e5ddae`.
- Verification bundle:
  `.revolvr/release-candidates/level1-v0.1.0-rc.3-a16ea1bdc1a4-verification/`.
- Verification inventory SHA-256:
  `006cb0b7f2215878e757ae8ee104bb1b88d5ae31b8661168bcf5c705d08353ff`.
- Build-instruction SHA-256:
  `2deaa06d380dfd7d86277e2090229ba1d212e1bad85c6b3db6a83a31036c1405`.
- Remote attestation artifact: ID `8431664217`, name
  `level1-v0.1.0-rc.3-attestation`, size 70,202,355 bytes, digest
  `sha256:8ac9f82795233b4808fc8c2fc895a11d1fb622e30272f73f90b1f68218c99cd1`,
  expiry `2026-10-16T16:17:29Z`. Its public archive endpoint returned HTTP 401,
  so no authenticated-download claim is made.

The controller independently reran both bundle verifiers, performed a third
non-local clean-clone rebuild, reproduced all three binaries byte-for-byte,
and ran an RC.3 no-model config/doctor smoke. The binary reported effective
config schema v8, source-writer timeout `32m0s`, heartbeat `10m40s`, required
window `32m0s`, and `Ready: true` without creating an operation.

## Prepared RC.3 No-Model Suite Authority

- Prepared root: `/tmp/revolvr-ext20-rc3.Qghf19/suite`.
- Authority SHA-256:
  `adc8095701e1fa6fdcd6180df93ea88ccac6f1d4d21cba1a751476a7a1ef3fb4`.
- Operation-plan SHA-256:
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`.
- Candidate output/hash/source and exact Codex package/output/hash agree with
  the RC.3 authority above. Both repositories are clean on `main`; `repo-a`
  HEAD is `07239693a95a1c73b61de77b74ade0a234e84075`, and `repo-b` HEAD is
  `2de82370e6c8bd3775e25da783fc7d6dcf0e0b5c`.
- The 11 plan rows name exactly ten unique tasks, and every task reports
  `Ready: true` with source-writer lock `timeout=32m0s`, heartbeat `10m40s`,
  and required window `32m0s`.
- There are zero runtime operation manifests, zero collector manifests, and
  zero aggregate entries. Whole-root content fingerprint
  `90424fc5544fd3be146eef965a95b5300a910036e0a19703672fc95f1fb756ae`
  and layout/metadata fingerprint
  `8fdd4272d36eefe1f9c99c4793bc47d51db5751ed12450b986755a875ae467b7`
  remained identical across independent inspection with
  `GIT_OPTIONAL_LOCKS=0`.
- The empty-confirmation guard refused in isolation before prepared-root or
  collector access. No `--live` argument or confirmation value was passed to
  the suite. The first root `/tmp/revolvr-ext20-rc3.5TQPha/suite` retained a
  Git optional index-refresh diagnostic and is not live authority.

## Rejected History To Preserve

- RC.1 source `ed65049fba6bf82852fd406ebc17afa90a953e3f` is rejected because
  autonomous CLI invocation omitted the working directory.
- RC.2 source `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec` is rejected because
  admitted effective lock authority was shorter than the supervisor window.
- RC.2 failed operation `ext20-3601e63c616b-01` historically retained a
  terminal `unsafe_or_ambiguous` bundle with zero model attempts, zero
  verification, and zero commits below `/tmp/revolvr-ext20-rc2.96ibla/suite`.
  That temporary root was already absent during the RC.3 attestation pass; do
  not reuse, recreate, or relabel it.
- Preserve every RC.1/RC.2 candidate and attestation ref, workflow, bundle,
  artifact, hash, retired root, and diagnostic. Do not relabel or reuse them.

## Remaining Ordered Work

1. Obtain a new explicit live confirmation for the exact command below.
2. Only after that confirmation, run all 11 planned operations
   and independently verify every EXT-17 manifest and EXT-20 aggregate.
3. Keep `EXT-20` unchecked and external use unapproved until every threshold
   passes. Tagging/release decision remains the later `EXT-21` task.

Exact confirmation-gated command to run only after that separate approval:

```bash
scripts/dogfood-external-level1-suite.sh --live --run-root /tmp/revolvr-ext20-rc3.Qghf19/suite --confirm-live-real-codex EXT20_LIVE_REAL_CODEX_MODEL_CALLS
```

## Session Rules

- Read `AGENTS.md`, this file, `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/DECISIONS.md` before acting.
- Use one new `codex exec` invocation per bounded task; never resume an old
  Codex session.
- Do exactly one task per pass and preserve unrelated or historical evidence.
- No live or nested model work unless the operator supplies the exact separate
  confirmation for that live command.
- The repository state is authoritative; this handoff is a concise resume
  pointer and does not replace the detailed evidence in `STATE.md` or durable
  decisions in `DECISIONS.md`.
