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
	printf 'RC.7 live-authority review requires a clean controller repository\n' >&2
	exit 1
}
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'RC.7 live-authority review requires exact local, fetched, and public main\n' >&2
	exit 1
}
[[ "$(git show -s --format='%H %T %P' f03258496621bbf8fd440a5c7293430a6ce44a22)" == "f03258496621bbf8fd440a5c7293430a6ce44a22 39ac014ebdcd56482b931fd79073429e51419df3 460c2fa31155dca28a4f9ce861c03fbad8949acc" ]] || {
	printf 'RC.7 pre-live review publication authority changed\n' >&2
	exit 1
}
git merge-base --is-ancestor f03258496621bbf8fd440a5c7293430a6ce44a22 HEAD || {
	printf 'RC.7 pre-live review publication is not in main ancestry\n' >&2
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
[[ "$(sha256sum .agent/TASKS.md | awk '{print $1}')" == 33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448 ]] || {
	printf 'RC.7 task backlog changed\n' >&2
	exit 1
}
[[ ! -e .revolvr/ext20-rc7-launch-records && ! -L .revolvr/ext20-rc7-launch-records ]] || {
	printf 'RC.7 launch-record root already exists\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.7 live-authority no-model review:
- Never use gh. Git operations must use raw Git; GitHub evidence must use read-only public REST. Do not commit or push; the controller will independently verify and publish later.
- Do exactly one final no-model admission review. Do not invoke --live, do not supply, reproduce, or embed the live confirmation token, do not create a launch record, and do not start Revolvr or any nested Codex/model operation. Do not construct another executable or activate a live command. Do not tag, release, approve external use, grant queue authority, or complete EXT-20.
- Require clean local, freshly fetched, and public main to agree. Require exact published pre-live review commit f03258496621bbf8fd440a5c7293430a6ce44a22, tree 39ac014ebdcd56482b931fd79073429e51419df3, parent 460c2fa31155dca28a4f9ce861c03fbad8949acc in local and fetched-main ancestry, and independently inspect its three-file durable-state delta.
- Keep agent-ext20-rc7-live-direct.sh exact at SHA-256 9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45 and scripts/check-ext20-rc7-live-direct.sh exact at SHA-256 c434380918c8ec726504126ac42b3a1338fa34741329ac60b28d44c03fc4f414. Inspect all launch-preflight, collision, status, interruption, and one-shot suite-process boundaries line by line without entering the live branch.
- Independently exercise syntax and safe missing, wrong, and multiple argument refusals without supplying live authority. Execute only ./agent-ext20-rc7-live-direct.sh --check and confirm its complete published authority preflight exits before launch-record creation or any model path.
- Reverify raw-Git/public-REST refs, absent tags, exact source/attestation/companion runs and jobs, sole artifact, both sealed candidate bundles, candidate and Codex identities, guarded script identities, the prepared suite and plan, repository/task/doctor/source-writer/sentinel state, zero operation/collector/model/receipt evidence, empty aggregate, and absent RC.7 launch-record root before and after.
- Preserve suite /home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite at ID ext20-14b2bf40212b and authority/plan/content SHA-256 c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e / 5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1 / 2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943 with all ten tasks pending and zero-attempt.
- Treat RC.1 through RC.6 and every historical ref, workflow, bundle, artifact, suite, operation, launch record, diagnostic, and evidence root as immutable rejected or failed history. Never execute, repair, delete, derive from, or reuse an RC.6 launcher or suite path.
- Preserve read-only RC.6 suite /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite, launch record /home/gernsback/source/revolvr/.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365, and terminal evidence /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite/evidence/repo-a/01-successful-source-change-1 at content-stream SHA-256 d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b / 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce / e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259 before and after.
- Run bash -n, safe refusal checks, the full published --check preflight, go test -count=1 ./..., and git diff --check. Keep .agent/TASKS.md exact at SHA-256 33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448 with EXT-20 unchecked.
- If every check passes, update only .agent/STATE.md, .agent/DECISIONS.md, and .agent/HANDOFF.md with a precise pass/fail recommendation for a later human/controller authorization decision. Make explicit that this review itself grants no live authority and leaves no live command active. Do not create any script or commit. Stop after the durable-state-only recommendation."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
