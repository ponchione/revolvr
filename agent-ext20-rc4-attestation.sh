#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.4 release-artifact attestation workflow:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, publish the attestation ref, and collect remote evidence.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest RC.4 local-candidate and exact-candidate remote-CI state before acting. Preserve RC.1, RC.2, and RC.3 and every historical workflow/ref/bundle/artifact/hash/suite/operation/run/diagnostic unchanged. Preserve exact RC.4 candidate ref refs/heads/level1-v0.1.0-rc.4 at 2546913e38ec273f64417dece2f91df78fd42fc2.
- Do exactly one bounded task: add the smallest separate RC.4 attestation workflow triggered only by a push to level1-v0.1.0-rc.4-attestation.
- The workflow must check out exact candidate source 2546913e38ec273f64417dece2f91df78fd42fc2 rather than trigger HEAD, install exact Go 1.26.5 with cache disabled, make two independent clean --no-local source clones with separate Go build and module caches, and build Linux/Darwin/FreeBSD amd64 using the settled release environment and flags, disabled CGO, trimpath, explicit clean VCS metadata, empty build ID, and main.version=0.1.0.
- Require byte-identical build pairs and exact hashes: Linux 98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe, Darwin 042563f350b71ec8cd5be1b49fc9d948383caa28087c0a5689bd6eb12f3808ab, and FreeBSD 128b9f8ced3038a51534da63b9d9ffbaa5ea7341e0ab8dd17102fba86084a8e6.
- Verify release version 0.1.0 plus Go/tool/path/compiler/trimpath/target/CGO/source/vcs.modified metadata, empty build IDs, and exact main.version authority for every artifact. Retain both binary sets, hashes, build metadata, version assertions, reproducibility evidence, and an exact authority manifest as one uploaded artifact named level1-v0.1.0-rc.4-attestation.
- Fail closed unless the workflow path, remote attestation ref, artifact name, and RC.4 attestation namespace are collision-free while the candidate ref remains exact.
- Validate workflow YAML structure, exact constants, embedded shell syntax, and the complete embedded shell locally from a detached exact-source clone. Do not start remote CI or alter any candidate source/ref.
- Keep EXT-20 unchecked. Update durable state, decisions, and handoff with files, local verification, preservation evidence, the collision-free raw-Git attestation ref to publish, and remaining remote artifact evidence."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
