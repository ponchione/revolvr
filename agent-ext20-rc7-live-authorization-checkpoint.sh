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
	printf 'RC.7 authorization checkpoint requires a clean controller repository\n' >&2
	exit 1
}
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'RC.7 authorization checkpoint requires exact local, fetched, and public main\n' >&2
	exit 1
}
[[ "$(git show -s --format='%H %T %P' 1a07077f24526b9202da55c2981911b7e0770a67)" == "1a07077f24526b9202da55c2981911b7e0770a67 1cb2ed843d1d5cf294487e8e404a030bb9f9838c b6351d108fc971dcfff5367267fb7eb1a3b00273" ]] || {
	printf 'RC.7 admission recommendation publication authority changed\n' >&2
	exit 1
}
git merge-base --is-ancestor 1a07077f24526b9202da55c2981911b7e0770a67 HEAD || {
	printf 'RC.7 admission recommendation is not in main ancestry\n' >&2
	exit 1
}
[[ "$(sha256sum agent-ext20-rc7-live-direct.sh | awk '{print $1}')" == 9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45 ]] || {
	printf 'RC.7 direct launcher changed\n' >&2
	exit 1
}
[[ "$(sha256sum .agent/TASKS.md | awk '{print $1}')" == 33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448 ]] || {
	printf 'RC.7 task backlog changed\n' >&2
	exit 1
}
[[ ! -e .revolvr/ext20-rc7-launch-records && ! -L .revolvr/ext20-rc7-launch-records ]] || {
	printf 'RC.7 launch-record root already exists\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.7 human-authorization checkpoint:
- Never use gh. Git operations must use raw Git; GitHub evidence must use read-only public REST. Do not commit or push; the controller will independently verify and publish later.
- Do exactly one no-model authorization-checkpoint task. Do not invoke --live, do not supply, reproduce, derive, or embed the live confirmation token, do not create a launch record, and do not start Revolvr or any nested Codex/model operation. Do not create or edit any executable. Do not tag, release, approve external use, grant queue authority, or complete EXT-20.
- Require clean local, freshly fetched, and public main to agree. Require exact published admission-recommendation commit 1a07077f24526b9202da55c2981911b7e0770a67, tree 1cb2ed843d1d5cf294487e8e404a030bb9f9838c, parent b6351d108fc971dcfff5367267fb7eb1a3b00273 in both local and fetched-main ancestry, and inspect its exact three-file durable-state delta.
- Treat the published PASS recommendation as technical readiness evidence only. The operator has not explicitly authorized a real model launch in the direction for this pass. Do not infer authorization from running this checkpoint, from prior review success, or from any retained command or transcript.
- Independently inspect the proposed one-shot scope: the fresh RC.7 suite contains eleven planned operations across ten unique tasks in two disposable repositories, including real Codex calls and the success, correction, needs-input, verification-failure, cancellation/restart, and safety-refusal scenarios. Record the material effects and retained-evidence behavior a human must knowingly authorize.
- Keep agent-ext20-rc7-live-direct.sh exact at SHA-256 9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45. Execute only its --check path. Confirm the complete preflight exits before launch-record reservation, status writes, suite execution, or model activity.
- Reverify prepared suite /home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite at ID ext20-14b2bf40212b and authority/plan/content SHA-256 c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e / 5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1 / 2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943, with ten pending zero-attempt tasks, zero evidence/aggregate, and no launch-record root.
- Preserve the protected RC.6 suite, launch record, and terminal evidence read-only at content-stream SHA-256 d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b / 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce / e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259. Never execute, repair, delete, derive from, or reuse RC.6.
- Run bash -n, safe argument refusals, the full published --check preflight, and git diff --check. Keep .agent/TASKS.md exact at SHA-256 33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448 with EXT-20 unchecked. Full Go tests are already exact published evidence and need not be rerun unless inspection finds a source or test delta.
- Update only .agent/STATE.md, .agent/DECISIONS.md, and .agent/HANDOFF.md. Record that technical admission passed but explicit human authorization is absent, state the exact bounded effects requiring consent, and leave no live command active. Do not formulate or expose the confirmation token or executable live command. Stop at the human authorization boundary."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
