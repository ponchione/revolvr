#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.7 remote attestation gate requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.7 remote attestation gate requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.7 remote artifact-attestation gate:
- Never use gh. Use raw Git for Git operations and read-only public GitHub REST responses for Actions evidence. Do not commit or push main; the controller will independently verify and publish durable-state changes afterward.
- Execution of this launcher is explicit authority for exactly one external mutation: collision-safe creation of refs/heads/level1-v0.1.0-rc.7-attestation at exact reviewed workflow commit 3cc6d527f889c7b933828fbd832d07b5291aee79. Do not create, update, or delete any other ref.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not change the candidate source/ref, prepare or reuse any suite, use a live confirmation token, tag, release, approve external use, grant queue authority, or complete EXT-20.
- Read the newest RC.7 local candidate, exact-candidate remote-CI, local attestation-workflow, and independent controller-review authority before acting. Treat RC.1 through RC.6 and all historical refs, workflows, bundles, artifacts, suites, operations, launch records, diagnostics, and retained evidence as immutable rejected or failed history.
- Require a clean worktree and index with local main exactly equal to freshly fetched origin/main. Require reviewed workflow commit 3cc6d527f889c7b933828fbd832d07b5291aee79, tree 1a35d15e75ce372d02f9499bd9634ca0f808f68d, parent b4c63dde894fec5805cd200164761c8f6e05b449, and workflow SHA-256 f3e06992e72029d80162c9b5901c398dbfb3c79cfeae43c0e72ddd28cff4ee13 to be published ancestors of origin/main.
- Before publication, independently reverify the complete RC.7 candidate and verification bundles at .revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb and its -verification sibling. Require candidate inventory/seal 7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba / 2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0, verification inventory/seal ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733 / 6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b, source commit f63cbe3989cb281652cf4eec3f92614fec98294d, source tree 43fc099d966cd6c06a74f00272c945fe3ca0a0f9, build-instructions hash ccf6cba57b00b3bdf1d50b074e4bbe9f13e3579493c22e87682f9d5952048ecd, and Linux/Darwin/FreeBSD hashes 1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a, fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086, and 13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe.
- Reverify exact candidate ref refs/heads/level1-v0.1.0-rc.7 at f63cbe3989cb281652cf4eec3f92614fec98294d and successful candidate CI run 30160277511 with exactly its ten recorded successful jobs. Reverify the reviewed workflow structure and extracted embedded-shell syntax plus retained 29-file local result /tmp/revolvr-ext20-rc7-attestation.Uq3syS. Require .agent/TASKS.md SHA-256 33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448 with EXT-20 unchecked.
- Verify the retained RC.6 terminal manifest before and after work. Preserve without executing or mutating the RC.6 suite /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite, launch record /home/gernsback/source/revolvr/.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365, and terminal evidence /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite/evidence/repo-a/01-successful-source-change-1 at content-stream SHA-256 values d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b, 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce, and e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259. RC.6 is retired and grants no authority.
- Immediately before publication, prove refs/heads/level1-v0.1.0-rc.7-attestation is absent locally and remotely, all *rc.7* tags are absent, the only existing RC.7 ref is the exact candidate ref, and the public artifact query has zero artifacts named level1-v0.1.0-rc.7-attestation. Fail closed on any collision or identity drift.
- Create only the attestation ref with raw Git using an empty-expected force-with-lease: git push --force-with-lease=refs/heads/level1-v0.1.0-rc.7-attestation: origin 3cc6d527f889c7b933828fbd832d07b5291aee79:refs/heads/level1-v0.1.0-rc.7-attestation. Read it back and require the exact SHA. Never force-update, delete, or move any existing ref.
- Locate the new push-triggered GitHub Actions run named Level 1 RC.7 candidate attestation through the public REST API. Require event push, head_branch level1-v0.1.0-rc.7-attestation, head_sha 3cc6d527f889c7b933828fbd832d07b5291aee79, exact workflow path, run attempt 1, and unambiguous identity. Poll with a finite bound until completion and require success.
- Require exactly one successful job named Rebuild and attest Level 1 RC.7 candidate at the exact head SHA. Require exactly one unexpired run artifact named level1-v0.1.0-rc.7-attestation with a nonempty ID, positive size, and sha256 digest. If an already configured read-only token permits archive download, verify the 29-file archive and all retained checksum, manifest, metadata, build-ID, version, and reproducibility authority; otherwise record the unauthenticated archive endpoint result and make no controller-side archive-byte claim.
- Also locate the companion .github/workflows/ci.yml push run at the exact attestation head and require its complete ten-job matrix to finish successfully. Record exact run, job, artifact IDs, URLs, timestamps, size, digest, and conclusions in durable state.
- Stop after remote ref/run/job/artifact and companion-CI evidence. Do not prepare a suite or any later gate in this pass. Update .agent/STATE.md, .agent/DECISIONS.md, and .agent/HANDOFF.md with exact remote readback, complete evidence, preservation checks, and the next separately bounded no-model RC.7 suite-preparation review gate. Keep .agent/TASKS.md unchanged."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
