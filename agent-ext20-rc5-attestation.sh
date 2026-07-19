#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 release-artifact attestation workflow:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, publish the attestation ref, and collect remote evidence.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest RC.5 local-candidate and exact-candidate remote-CI state before acting. Preserve RC.1 through RC.4 as immutable rejected history, including terminal RC.4 suite /tmp/revolvr-ext20-rc4.DGg1pW/suite and operation ext20-2bd21aea4f72-01. Preserve every historical workflow, ref, bundle, artifact, hash, suite, operation, run, diagnostic, and sentinel unchanged.
- Preserve exact RC.5 candidate ref refs/heads/level1-v0.1.0-rc.5 at 19c1ef4b6a610016487880aa8ad69ec0204bd4f7 and exact successful push-triggered CI run 29697069305 with its ten recorded jobs. Reverify both sealed RC.5 bundle inventories before changing the worktree.
- Do exactly one bounded task: add the smallest separate RC.5 attestation workflow at .github/workflows/level1-rc5-candidate-attestation.yml, triggered only by a push to level1-v0.1.0-rc.5-attestation.
- The workflow must check out exact candidate source 19c1ef4b6a610016487880aa8ad69ec0204bd4f7 and tree 2fb39c93694e72d986e7a8a849a542fc1bf1728d rather than trigger HEAD, install exact Go 1.26.5 with cache disabled, make two independent clean --no-local source clones with separate Go build and module caches, and build Linux/Darwin/FreeBSD amd64 using the settled release environment and flags, disabled CGO, trimpath, explicit clean VCS metadata, empty build ID, and main.version=0.1.0.
- Require byte-identical build pairs and exact hashes: Linux 1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043, Darwin a0ba1e05f76d92c1d20577c897a37bc2b4a3252a4e0fb10ef9d736f25b07645d, and FreeBSD f9b6da20be9497c5eb772f7b40945fceedc064ecb6e081809c9510d71462e2d6.
- Verify release version 0.1.0 plus Go/tool/path/compiler/trimpath/target/CGO/source/vcs.modified metadata, empty build IDs, and exact main.version authority for every artifact. Retain both binary sets, hashes, build metadata, version assertions, reproducibility evidence, and an exact authority manifest as one uploaded artifact named level1-v0.1.0-rc.5-attestation.
- Fail closed unless the workflow path, remote attestation ref, artifact name, RC.5 tag namespace, and RC.5 attestation namespace are collision-free while the candidate ref remains exact. The existing candidate ref is the only allowed RC.5 ref before later attestation publication.
- Validate workflow YAML structure, exact constants, embedded shell syntax, and the complete embedded shell locally from a detached exact-source clone. Do not start remote CI or alter any candidate source/ref.
- Keep EXT-20 unchecked. Update durable state, decisions, and handoff with files, local verification, preservation evidence, the collision-free raw-Git attestation ref to publish, and remaining remote artifact evidence. This pass grants no workflow commit/push, attestation ref, remote run/artifact, suite, model call, tag, release, external-use approval, or EXT-20 completion."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
