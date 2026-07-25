#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

git fetch --no-tags origin main
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.7 pre-live review requires a clean controller repository\n' >&2
	exit 1
}
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'RC.7 pre-live review requires exact local, fetched, and public main\n' >&2
	exit 1
}
git merge-base --is-ancestor d2e7d8ddda66db7685b021dcc33b94766ec00204 HEAD || {
	printf 'RC.7 direct-launch publication authority is not in main ancestry\n' >&2
	exit 1
}
[[ "$(sha256sum agent-ext20-rc7-live-direct.sh | awk '{print $1}')" == 9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45 ]] || {
	printf 'RC.7 direct launcher changed\n' >&2
	exit 1
}
[[ "$(sha256sum scripts/check-ext20-rc7-live-direct.sh | awk '{print $1}')" == c434380918c8ec726504126ac42b3a1338fa34741329ac60b28d44c03fc4f414 ]] || {
	printf 'RC.7 focused checker changed\n' >&2
	exit 1
}
[[ ! -e .revolvr/ext20-rc7-launch-records && ! -L .revolvr/ext20-rc7-launch-records ]] || {
	printf 'RC.7 launch-record root already exists\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.7 pre-live no-model review:
- Never use gh. Git operations must use raw Git; GitHub evidence must use read-only public REST. Do not commit or push; the controller will independently verify and publish later.
- This is exactly one independent pre-live review task. Do not construct or start a live gate, do not invoke --live, do not supply or reproduce the live confirmation token, do not create a launch record, and do not start Revolvr or any nested Codex/model operation. Do not tag, release, approve external use, grant queue authority, or complete EXT-20.
- Require a clean worktree and index with local main equal to freshly fetched origin/main and public main. Require published direct-launch record d2e7d8ddda66db7685b021dcc33b94766ec00204, tree 701639e624fc86a0a18c77cf19dc064f8e7bd511, parent 6931b3ef790a6f0375944043596cb591cf589f2a in both local and fetched-main ancestry.
- Independently inspect the complete direct launcher and focused checker. Keep agent-ext20-rc7-live-direct.sh exact at SHA-256 9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45 and scripts/check-ext20-rc7-live-direct.sh exact at SHA-256 c434380918c8ec726504126ac42b3a1338fa34741329ac60b28d44c03fc4f414. Do not edit either file.
- Reverify missing, wrong, and multiple argument refusal without passing the live token. The focused checker was intentionally a construction-time dirty-worktree checker; inspect its coverage but do not mistake its expected clean-controller refusal for a live-gate defect.
- Execute only the published check-only path ./agent-ext20-rc7-live-direct.sh --check. Confirm it completes the clean published-main, remote ref/run/job/artifact, sealed-bundle/script, prepared-root, candidate/Codex, repository/task/doctor/source-writer/sentinel, zero-evidence/aggregate, absent-launch-record, task-backlog, and RC.6 preservation preflight without runtime mutation or a model call.
- Bind suite root /home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite, suite ID ext20-14b2bf40212b, authority/plan/content SHA-256 c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e / 5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1 / 2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943. Require zero operation/collector/model evidence, empty aggregate, ten pending zero-attempt doctor-ready tasks, and no RC.7 launch-record root before and after.
- Treat RC.1 through RC.6 and every historical ref, workflow, bundle, artifact, suite, operation, launch record, diagnostic, and evidence root as immutable rejected or failed history. Never execute any RC.6 launcher or suite path and never repair, delete, mutate, derive from, or reuse RC.6.
- Preserve read-only RC.6 suite /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite, launch record /home/gernsback/source/revolvr/.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365, and terminal evidence /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite/evidence/repo-a/01-successful-source-change-1 at content-stream SHA-256 d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b, 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce, and e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259 before and after.
- Run bash -n on the launcher and checker, the safe refusal checks, the complete published --check preflight, go test -count=1 ./..., and git diff --check. Keep .agent/TASKS.md unchanged at SHA-256 33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448 with EXT-20 unchecked.
- If and only if every no-model check passes, update only .agent/STATE.md, .agent/DECISIONS.md, and .agent/HANDOFF.md with exact evidence and return to the controller for a separate authority decision. Do not make any live command active and do not create another continuation script in this pass. Stop after the no-model review."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
