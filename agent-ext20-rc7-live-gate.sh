#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.7 live-gate construction requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.7 live-gate construction requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.7 live-gate construction pass:
- Never use gh. Git operations must use raw Git; GitHub evidence must use read-only public REST. Do not commit or push; the controller will independently verify and publish later.
- This is a construction-only, no-Revolvr-model pass. Do not invoke --live, do not run any launcher with a live confirmation token, do not create a launch record, and do not start a Revolvr or nested Codex/model operation. Do not tag, release, approve external use, grant queue authority, or complete EXT-20.
- Require a clean worktree and index with local main exactly equal to freshly fetched origin/main. Require published prepared-suite authority commit 83078e8467f00439956252955d5c130d51f34214, tree 31e680996695e1bc71d38e1216a471250009fb0d, and parent 29ca11e24f2cc8832615fe5274d79c151d1eb5c0 in both ancestries.
- Read the newest RC.7 candidate, remote-CI, attestation, and prepared-suite authority first. Reverify both sealed RC.7 bundles, exact remote refs/runs/jobs/artifact, all historical refs/tags, and unchanged .agent/TASKS.md before and after work.
- Treat RC.1 through RC.6 and every historical ref, workflow, bundle, artifact, suite, operation, launch record, diagnostic, and evidence root as immutable rejected or failed history. Never execute any RC.6 launcher or suite path and never repair, delete, mutate, derive from, or reuse RC.6.
- Preserve read-only RC.6 suite /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite, launch record /home/gernsback/source/revolvr/.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365, and terminal evidence /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite/evidence/repo-a/01-successful-source-change-1 at content-stream SHA-256 values d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b, 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce, and e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259.
- Do exactly one bounded task: construct a fail-closed RC.7 direct launcher at agent-ext20-rc7-live-direct.sh and its no-model focused checker at scripts/check-ext20-rc7-live-direct.sh. Adapt the established direct-launch architecture, but bind only the exact RC.7 authorities below and do not execute the live path.
- Bind suite root /home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite, suite ID ext20-14b2bf40212b, authority/plan/content SHA-256 c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e / 5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1 / 2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943, and repository heads 22bc5fd5ea1469fb76afef6425964f0b0c7f70bb / f92954597d8bd35372ee181c959be9a5fc637429.
- Bind candidate source f63cbe3989cb281652cf4eec3f92614fec98294d, Linux SHA-256 1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a, exact candidate bundle, Codex 0.144.4 SHA-256 134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477, suite-script SHA-256 8957ac1c8d9ad1901ccb707bc7ca270e670de83572ec5b96933291a99b838317, and collector SHA-256 2aa507930a12f4040fc8e1e359968b67d2be9cfa6e92aa65d9c8ce0577959cdd.
- Bind exact candidate ref refs/heads/level1-v0.1.0-rc.7, attestation ref refs/heads/level1-v0.1.0-rc.7-attestation at 3cc6d527f889c7b933828fbd832d07b5291aee79, artifact 8621008768 named level1-v0.1.0-rc.7-attestation with digest sha256:ae87472ef86b5d25cca5df333f057f10d77cf74cd7f332f30d6770745bbf6356, source CI 30160277511, attestation run/job 30163857880 / 89693466274, and companion CI 30163853353.
- The direct launcher must accept only --check or the exact existing live confirmation token. Missing/wrong arguments must fail before authority checks. Check mode must perform the complete clean published-main, ref/artifact, bundle/script, prepared-root, candidate/Codex, repository/task/doctor/source-writer/sentinel, zero-evidence/aggregate, absent-launch-record, and RC.6 preservation preflight, then exit without mutation or model calls.
- A future separately confirmed live path may reserve exactly one collision-free ignored RC.7 launch record only after the complete preflight, retain pre-start authority and terminal/interruption status, and run the guarded suite exactly once. Any existing RC.7 launch-record root or evidence must fail closed; no retry or reuse path is allowed.
- The focused checker must exercise syntax, missing/wrong confirmation refusal, dirty/unpublished check-only refusal during construction, definition loading without main, isolated launch-record collision handling, and before/after RC.7 suite plus RC.6 protected-stream hashes. It must prove zero evidence, aggregate, launch record, or model operation occurred. Never invoke the future live path.
- Run bash -n on both new files, the focused checker, go test -count=1 ./..., and git diff --check. Keep the exact suite root byte-identical and .agent/TASKS.md unchanged at SHA-256 33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448 with EXT-20 unchecked.
- Update .agent/STATE.md, .agent/DECISIONS.md, and .agent/HANDOFF.md with exact files, hashes, tests, and the next fresh independent no-model review/publication gate. Do not make a live command the active next command in this pass. Stop after construction and no-model checks."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
