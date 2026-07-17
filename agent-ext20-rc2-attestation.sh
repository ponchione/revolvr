#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.2 remote artifact attestation:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest RC.2 local-candidate and exact-candidate remote-CI state before acting.
- Preserve RC.1, its workflows, refs, hashes, bundles, and failed live evidence unchanged.
- Do exactly one bounded task: add the smallest separate RC.2 attestation workflow triggered only by a push to level1-v0.1.0-rc.2-attestation.
- The workflow must check out exact source commit eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec rather than the trigger HEAD, install exact Go 1.26.5, build Linux/Darwin/FreeBSD amd64 with the settled EXT-18 flags in two independent clean passes, compare each pair byte-for-byte, and require SHA-256 values 06c1258a947def8c53e03bfd79944bb002351358fc8dfecd35682ab7532b5010, 05a15786dd1617d77ec671f420075922f6f9a78bf03de1245f03008f0960dee1, and 5891c88e1e13f5a0a0e3452c15221981a187652c2e563a7b8b218b63c07d2a29.
- It must verify release version 0.1.0, Go/tool/target/CGO/source/vcs.modified metadata for every artifact and retain the binaries, hashes, build metadata, and exact authority manifest as one uploaded artifact.
- Validate workflow syntax and exact constants locally without starting remote CI. Do not alter the candidate source commit or mark EXT-20 complete.
- Update durable state with files, verification, the collision-free raw-Git attestation ref to publish, and remaining remote evidence."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
