# Agent Handoff

Updated: 2026-07-26

## Resume Point

The planner lifecycle-contract remediation passed independent and controller
review and is published on raw-Git `origin/main` as exact source commit
`a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
`2c8ee9f6b4283410547a9f99972e25eac06c9e33`, parent
`a9b4f50465466fee61e13f131ae6679e3d8b4729`. Local, fetched, and public
`main` readback matched that commit before the separate RC.8 continuation was
created.

The accepted remediation keeps the safe Go validators strict, makes planner
prompts and the Structured Outputs schema express the exact lifecycle fields,
and treats schema-required empty evidence arrays as equal to omitted empty
slices after canonical state round-trips. Focused, race, production
strict-fake, full-suite, formatting, syntax, diff-hygiene, and pre/post RC.7
bundle verification passed.

## Exact Next-Session Resume

From `/home/gernsback/source/revolvr`, run exactly:

```bash
./agent-ext20-rc8.sh
```

The executable launcher SHA-256 is
`d27842590accf418e20f9ddd5ca048e6b299776ad7d8b17f02fc933c51003284`.
It starts one fresh Codex pass with approval/sandbox bypass to construct and
locally verify `level1-v0.1.0-rc.8` from the exact published source above.
Remain aware that the fresh Codex process inherits host access and may use
network/toolchain resources for local build and vulnerability verification.

This command does not run Revolvr dogfood, create a suite, make a live product
model call, publish a ref, commit, push, tag, release, approve external use,
grant queue authority, or complete `EXT-20`. It must stop after local candidate
construction and preparation of an inert independent-review continuation.

## Immutable Failure Boundary

- RC.7 suite / launch record / terminal-evidence content-stream SHA-256:
  `ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a` /
  `deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c` /
  `6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f`.
- RC.6 suite / launch record / terminal-evidence content-stream SHA-256:
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b` /
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce` /
  `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.

Never execute, retry, repair, delete, mutate, derive from, or reuse any RC.6 or
RC.7 wrapper, launcher, suite, operation, launch record, or evidence root.
The RC.7 terminal checksum manifests may be reverified read-only.

## Next Ordered Work

1. Run `./agent-ext20-rc8.sh` for one bounded local candidate-construction
   pass.
2. Run the inert independent local-review continuation produced by that pass.
3. Return for separate controller decisions about candidate publication,
   remote CI, attestation, suite preparation, and human live authority. None is
   granted in advance.

## Session Rules

- Read `AGENTS.md`, this file, `.agent/TASKS.md`, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and `.agent/LOOP_PROMPT.md` before acting.
- Use a new `codex exec` invocation for each bounded task; never resume an old
  session or rely on old transcripts.
- Do exactly one task per pass, preserve unrelated changes and immutable
  evidence, and never use `gh`.
- `EXT-20` remains unchecked. The repository is durable memory; this file is
  only the active resume pointer.
