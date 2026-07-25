#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.7 attestation construction requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.7 attestation construction requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.7 artifact-attestation workflow:
- Never use gh. Use raw Git for Git reads and read-only GitHub REST for remote evidence. Do not commit or push; the controller will independently review and publish later.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not publish an attestation ref, request a remote workflow run or artifact, prepare a suite, use any live confirmation token, tag, release, approve external use, grant queue authority, or complete EXT-20.
- Require published remote-CI record commit a0f5b069ea5189b11f1d4de71c93e825814adee5 to be an ancestor of exact local/remote main. Read the newest RC.7 local-candidate, independent-review, and exact-candidate remote-CI authority before acting.
- Preserve exact candidate ref refs/heads/level1-v0.1.0-rc.7 at source f63cbe3989cb281652cf4eec3f92614fec98294d, tree 43fc099d966cd6c06a74f00272c945fe3ca0a0f9, and successful push-triggered CI run 30160277511 with its ten recorded jobs. Reverify both complete sealed RC.7 bundles before changing the worktree.
- Require candidate and verification inventory/seals 7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba / 2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0 and ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733 / 6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b, build-instructions SHA-256 ccf6cba57b00b3bdf1d50b074e4bbe9f13e3579493c22e87682f9d5952048ecd, and Linux/Darwin/FreeBSD artifact SHA-256 values 1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a, fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086, and 13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe.
- Treat RC.1 through RC.6 and all historical refs, workflows, bundles, artifacts, suites, operations, launch records, diagnostics, and retained evidence as immutable rejected or failed history. Verify the RC.6 terminal manifest and preserve the RC.6 suite, launch-record, and terminal-evidence content-stream SHA-256 values d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b, 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce, and e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259 before and after work. Never execute or mutate any RC.6 launcher, suite, or evidence.
- Do exactly one bounded task: add the smallest separate RC.7 attestation workflow at .github/workflows/level1-rc7-candidate-attestation.yml, triggered only by a push to level1-v0.1.0-rc.7-attestation.
- The workflow must check out exact candidate source f63cbe3989cb281652cf4eec3f92614fec98294d and tree 43fc099d966cd6c06a74f00272c945fe3ca0a0f9 rather than trigger HEAD; install exact Go 1.26.5 with cache disabled; create two independent clean --no-local source clones with separate Go build and module caches; and build Linux, Darwin, and FreeBSD amd64 with module-readonly mode, disabled CGO, trimpath, explicit clean VCS metadata, empty build ID, and main.version=0.1.0.
- Require byte-identical build pairs and exact hashes: Linux 1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a, Darwin fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086, and FreeBSD 13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe.
- Verify release version 0.1.0 plus Go/tool/path/compiler/trimpath/target/CGO/source/vcs.modified metadata, empty build IDs, and exact main.version authority for every artifact. Retain both binary sets, hashes, build metadata, version assertions, reproducibility evidence, and an exact authority manifest as one uploaded artifact named level1-v0.1.0-rc.7-attestation.
- Fail closed unless the workflow path, remote attestation ref, artifact name, RC.7 tag namespace, and RC.7 attestation namespace are collision-free while the candidate ref remains exact. The existing candidate ref is the only allowed RC.7 ref before later attestation publication.
- Validate workflow YAML structure, exact constants, embedded shell syntax, and the complete unmodified embedded shell locally from a detached exact-source clone. Reverify bundle seals, remote candidate/CI authority, historical refs, and retained evidence afterward.
- Keep .agent/TASKS.md unchanged with EXT-20 unchecked. Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with files, workflow hash, local execution evidence, preservation checks, and the next independent review/publication gate. Stop after this one task."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
