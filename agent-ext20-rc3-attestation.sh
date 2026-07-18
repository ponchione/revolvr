#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.3 remote artifact attestation:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest RC.3 local-candidate and exact-candidate remote-CI state before acting.
- Preserve RC.1 and RC.2, both existing attestation workflows/refs/artifacts, every old bundle/hash/root/diagnostic, failed RC.2 suite /tmp/revolvr-ext20-rc2.96ibla/suite, and exact RC.3 candidate ref unchanged.
- Do exactly one bounded task: add the smallest separate RC.3 attestation workflow triggered only by a push to level1-v0.1.0-rc.3-attestation.
- The workflow must check out exact candidate source a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c rather than trigger HEAD, install exact Go 1.26.5 with cache disabled, make two independent clean --no-local source clones with separate Go build/module caches, and build Linux/Darwin/FreeBSD amd64 using the settled EXT-18 environment, flags, empty build ID, and main.version=0.1.0.
- Require byte-identical pass pairs and exact hashes: Linux 9e9c13f43977edf49e7e6385c595aa20a01c16308ddfea1c30455ea88252ae9b, Darwin ee6db29cfcbcbd2e645184927fc5d7348ed924d6036a46d7b23eae55b5b43fd4, FreeBSD 6a42dc423ab1975e8ea4296f56be6c9a3773d9ccfda1c57244a50261e64f368a.
- Verify release version 0.1.0 plus Go/tool/path/compiler/trimpath/target/CGO/source/vcs.modified metadata and exact main.version authority for every artifact. Retain both binary sets, all hashes/build metadata/version assertions, reproducibility evidence, and exact authority manifest as one uploaded artifact.
- Validate workflow YAML structure, exact constants, embedded shell syntax, and the complete embedded shell locally from a detached exact-source clone. Do not start remote CI or alter any candidate source/ref.
- Keep EXT-20 unchecked. Update durable state with files, local verification, preservation evidence, the collision-free raw-Git attestation ref to publish, and remaining remote artifact evidence."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
