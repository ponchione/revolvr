#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.7 suite preparation requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.7 suite preparation requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.7 no-model suite preparation:
- Never use gh. Git operations must use raw Git; GitHub evidence must use read-only public REST. Do not commit or push; the controller will independently verify and publish later.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not pass --live, create a live launcher or launch record, use a live confirmation token, tag, release, approve external use, grant queue authority, or complete EXT-20.
- Require a clean worktree and index with local main exactly equal to freshly fetched origin/main. Require published remote-attestation record c17a8f08efe1fedb1edcdc5f98b6d03ebc0e5a3c to be an ancestor of both.
- Read the newest RC.7 local-candidate, exact-candidate remote-CI, local workflow, controller review, and remote artifact-attestation state before acting. Reverify complete candidate and verification bundles at .revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb and its -verification sibling.
- Require candidate inventory/seal 7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba / 2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0, verification inventory/seal ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733 / 6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b, source f63cbe3989cb281652cf4eec3f92614fec98294d, tree 43fc099d966cd6c06a74f00272c945fe3ca0a0f9, build-instructions SHA-256 ccf6cba57b00b3bdf1d50b074e4bbe9f13e3579493c22e87682f9d5952048ecd, and Linux/Darwin/FreeBSD SHA-256 values 1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a, fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086, and 13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe.
- Reverify exact candidate ref refs/heads/level1-v0.1.0-rc.7 at f63cbe3989cb281652cf4eec3f92614fec98294d, source CI run 30160277511, exact attestation ref refs/heads/level1-v0.1.0-rc.7-attestation at 3cc6d527f889c7b933828fbd832d07b5291aee79, workflow SHA-256 f3e06992e72029d80162c9b5901c398dbfb3c79cfeae43c0e72ddd28cff4ee13, dedicated run/job 30163857880 / 89693466274, artifact 8621008768 named level1-v0.1.0-rc.7-attestation with digest sha256:ae87472ef86b5d25cca5df333f057f10d77cf74cd7f332f30d6770745bbf6356, and successful companion ten-job CI run 30163853353.
- Treat RC.1 through RC.6 and every historical ref, workflow, bundle, artifact, suite, operation, launch record, diagnostic, and retained evidence root as immutable rejected or failed history. Never rerun, repair, reconcile, relabel, delete, mutate, or reuse any of it.
- In particular, never execute or mutate RC.6 suite /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite, launch record /home/gernsback/source/revolvr/.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365, or terminal evidence /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite/evidence/repo-a/01-successful-source-change-1. Verify its retained terminal manifest read-only and preserve their content-stream SHA-256 values d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b, 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce, and e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259 before and after work.
- Do exactly one bounded task: update the guarded external Level-1 suite from retired RC.6 candidate authority to exact RC.7 candidate authority, verify it without model calls, and prepare exactly one new collision-free persistent RC.7 suite root for later independent review.
- In scripts/dogfood-external-level1-suite.sh change only the exact candidate authority constants: source f63cbe3989cb281652cf4eec3f92614fec98294d, Linux SHA-256 1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a, and bundle .revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb. Preserve release output revolvr 0.1.0, exact Codex 0.144.4 executable authority, model, reasoning effort, plan, schemas, scenarios, thresholds, configuration, confirmation guard, collector behavior, and every unrelated byte.
- Verify shell syntax, exact three-constant source diff, both complete sealed RC.7 bundles, suite --static, and collector fixture/manifest behavior. Run go test -count=1 ./... and git diff --check.
- Create exactly one persistent parent beneath ignored repository runtime state using mkdir -p \"$ROOT/.revolvr\" and mktemp -d \"$ROOT/.revolvr/ext20-rc7.XXXXXX\", then run scripts/dogfood-external-level1-suite.sh --prepare --run-root <new-parent>/suite --install-codex. Preparation may install the exact package but must start no model. Do not use /tmp and do not reuse or derive from RC.6.
- Independently inspect the new suite authority, plan and content hashes; exact candidate and Codex identities; effective 32-minute source-writer authority; exact 11-row plan and ten unique pending doctor-ready tasks; intact sentinels; both clean disposable repositories; zero operation and collector manifests; empty aggregate; and absence of any launch record. Verify the new suite with --verify-suite only; never point current suite tooling at RC.6.
- Retain and report the exact new RC.7 suite path, suite ID, authority/plan/content hashes, repository heads, and all no-model verification. Do not record a live command as the active next command and do not prepare a live launcher in this pass.
- Reverify all RC.7 remote authority, all historical refs/tags, both sealed bundles, and the three protected RC.6 content streams afterward. Keep .agent/TASKS.md byte-for-byte unchanged at SHA-256 33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448 with EXT-20 unchecked.
- Update .agent/STATE.md, .agent/DECISIONS.md, and .agent/HANDOFF.md with the exact tracked delta, verification, prepared-root evidence, and the next fresh independent no-model review/publication gate. Stop after this one task."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
