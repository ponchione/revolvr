#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.3 replacement candidate:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass.
- Preserve RC.1 and RC.2, all four candidate/attestation refs and workflows, every old bundle/artifact/hash, both earlier retired roots, and failed RC.2 suite root /tmp/revolvr-ext20-rc2.96ibla/suite unchanged.
- Read the newest source-lock defect/fix state first. Verify exact source commit a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c and tree 23c0d27fc62be5f41feb45192e74f1df8ecff3fa, and verify the focused source-lock regression before candidate construction.
- Do exactly one bounded task: construct and locally verify collision-free candidate level1-v0.1.0-rc.3 from that exact clean source commit. The controller-created untracked helper agent-ext20-rc3.sh is not candidate source and must not be added to the candidate commit or clean clones.
- Follow the settled EXT-18 reproducible procedure with exact Go 1.26.5 and release version 0.1.0: verify the Go 1.22 source floor, run required tests/vet/module/vulnerability checks, build Linux/Darwin/FreeBSD amd64 with the settled environment and flags, verify embedded version/source/tool/target/CGO/vcs.modified authority, and require two independent non-local clean-clone builds to be byte-identical.
- Retain a new immutable RC.3 candidate bundle and separate verification evidence with exact build instructions, source/tree/tool identities, artifact hashes, metadata, inventories, and focused source-lock proof. Check local and remote RC.3 ref/bundle/run-root collisions before construction.
- Do not publish a candidate ref, request remote CI, add an attestation workflow, prepare another EXT-20 suite, tag a release, or mark EXT-20 complete.
- Update durable state and decisions with exact RC.3 source/tree, hashes, bundle paths/inventories, verification, preservation evidence, and the next collision-free raw-Git candidate-ref/remote-CI step."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
