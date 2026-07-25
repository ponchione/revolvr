#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.6 attestation construction requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.6 attestation construction requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.6 artifact-attestation workflow:
- Never use gh. Use raw Git for Git reads. Do not commit or push; the controller will independently review and publish later.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not publish an attestation ref, request a remote workflow run or artifact, prepare a suite, use any historical live confirmation token, tag, release, approve external use, grant queue authority, or complete EXT-20.
- Require published remote-CI record commit 716c9a2293555f9abee80d384a673be426fe2ce0 to be an ancestor of exact local/remote main. Read the newest RC.6 local-candidate, independent-review, and exact-candidate remote-CI authority before acting.
- Preserve exact candidate ref refs/heads/level1-v0.1.0-rc.6 at source 73f1f81f1c51d927114f19818a18161d0fcb8541, tree 7c9753461a08b25915f4f53533d91e57d40a20ca, and successful push-triggered CI run 30153462797 with its ten recorded jobs. Reverify both complete sealed RC.6 bundles before changing the worktree.
- Require candidate and verification inventory/seals 30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1 / d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88 and 9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf / f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f, build-instructions SHA-256 94d291ec80db7427bddc1db57cac147c5d061ca3dbdbdd038259e1da3505a906, and Linux/Darwin/FreeBSD artifact SHA-256 values f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d, 596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7, and 60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344.
- Treat RC.1 through RC.5 and all historical refs, workflows, bundles, artifacts, suites, operations, launch records, diagnostics, and retained evidence as immutable rejected history. Preserve the RC.5 suite/evidence/launch-record content-stream SHA-256 values 875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c, 9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76, and f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8 before and after work. Require agent-ext20-rc5-live-direct.sh to remain permanently failed closed.
- Do exactly one bounded task: add the smallest separate RC.6 attestation workflow at .github/workflows/level1-rc6-candidate-attestation.yml, triggered only by a push to level1-v0.1.0-rc.6-attestation.
- The workflow must check out exact candidate source 73f1f81f1c51d927114f19818a18161d0fcb8541 and tree 7c9753461a08b25915f4f53533d91e57d40a20ca rather than trigger HEAD; install exact Go 1.26.5 with cache disabled; create two independent clean --no-local source clones with separate Go build and module caches; and build Linux, Darwin, and FreeBSD amd64 with module-readonly mode, disabled CGO, trimpath, explicit clean VCS metadata, empty build ID, and main.version=0.1.0.
- Require byte-identical build pairs and exact hashes: Linux f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d, Darwin 596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7, and FreeBSD 60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344.
- Verify release version 0.1.0 plus Go/tool/path/compiler/trimpath/target/CGO/source/vcs.modified metadata, empty build IDs, and exact main.version authority for every artifact. Retain both binary sets, hashes, build metadata, version assertions, reproducibility evidence, and an exact authority manifest as one uploaded artifact named level1-v0.1.0-rc.6-attestation.
- Fail closed unless the workflow path, remote attestation ref, artifact name, RC.6 tag namespace, and RC.6 attestation namespace are collision-free while the candidate ref remains exact. The existing candidate ref is the only allowed RC.6 ref before later attestation publication.
- Validate workflow YAML structure, exact constants, embedded shell syntax, and the complete unmodified embedded shell locally from a detached exact-source clone. Reverify bundle seals, remote candidate/CI authority, historical refs, and retained evidence afterward.
- Keep .agent/TASKS.md unchanged with EXT-20 unchecked. Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with files, workflow hash, local execution evidence, preservation checks, and the next independent review/publication gate. Stop after this one task."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
