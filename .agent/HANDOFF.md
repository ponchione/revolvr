# Active Work Handoff

Updated: 2026-07-18

## Resume Point

The first unchecked backlog task remains `EXT-20`, the quantitative Level-1
real-Codex dogfood gate. Do not restart RC.1 or RC.2 work and do not run a live
suite yet. RC.3 local reproducibility and exact-source remote CI have passed;
the next bounded task is the separate RC.3 artifact-attestation workflow.

Run this from the repository root:

```bash
./agent-ext20-rc3-attestation.sh
```

This starts one fresh `codex exec` pass. It must add and locally validate only
the RC.3 attestation workflow, make no model call through Revolvr, and perform
no commit or push. When it finishes, return control to the controller for
independent verification, raw-Git commit/push, attestation-ref publication,
and remote-run monitoring.

## Git And Release Authority

- Last evidence commit before this handoff: `8b37a6e` (`Record RC.3 remote
  candidate CI`). The handoff and helper are committed on top of it; use
  `git rev-parse HEAD` for the exact resumed `main` tip.
- RC.3 candidate source: `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`.
- RC.3 source tree: `23c0d27fc62be5f41feb45192e74f1df8ecff3fa`.
- Remote candidate ref: `refs/heads/level1-v0.1.0-rc.3` at the exact source
  commit above.
- Candidate CI: run `29642126354`, exact source SHA, conclusion `success`, all
  ten mandatory jobs passed.
- RC.3 attestation ref `refs/heads/level1-v0.1.0-rc.3-attestation` is absent at
  the handoff boundary. It must remain collision-free until the reviewed
  workflow commit is ready.
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

The controller independently reran both bundle verifiers, performed a third
non-local clean-clone rebuild, reproduced all three binaries byte-for-byte,
and ran an RC.3 no-model config/doctor smoke. The binary reported effective
config schema v8, source-writer timeout `32m0s`, heartbeat `10m40s`, required
window `32m0s`, and `Ready: true` without creating an operation.

## Rejected History To Preserve

- RC.1 source `ed65049fba6bf82852fd406ebc17afa90a953e3f` is rejected because
  autonomous CLI invocation omitted the working directory.
- RC.2 source `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec` is rejected because
  admitted effective lock authority was shorter than the supervisor window.
- RC.2 failed operation `ext20-3601e63c616b-01` retained a terminal
  `unsafe_or_ambiguous` bundle with zero model attempts, zero verification,
  and zero commits at
  `/tmp/revolvr-ext20-rc2.96ibla/suite/evidence/repo-a/01-successful-source-change-1`.
- Preserve every RC.1/RC.2 candidate and attestation ref, workflow, bundle,
  artifact, hash, retired root, and diagnostic. Do not relabel or reuse them.

## Remaining Ordered Work

1. Run `agent-ext20-rc3-attestation.sh` and independently review its local
   workflow execution and preservation evidence.
2. If it passes, commit and push the workflow/helper/state on `main` with raw
   Git, then publish only the collision-free RC.3 attestation ref at that
   reviewed workflow commit.
3. Require the remote attestation job and artifact upload to pass; record the
   exact run, checkout SHA, job conclusions, artifact ID/size/digest/expiry,
   and any authenticated-download limitation.
4. Update the guarded Level-1 suite to RC.3 and prepare a new collision-free
   no-model root. Do not reuse the failed RC.2 suite.
5. Only after a new explicit live confirmation, run all 11 planned operations
   and independently verify every EXT-17 manifest and EXT-20 aggregate.
6. Keep `EXT-20` unchecked and external use unapproved until every threshold
   passes. Tagging/release decision remains the later `EXT-21` task.

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
