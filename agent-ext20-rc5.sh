#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 replacement candidate:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest RC.4 terminal lifecycle failure, lifecycle-authority remediation, independent review, and publication state before acting.
- Preserve RC.1, RC.2, RC.3, and RC.4 as immutable rejected history. Never retry, reconcile, relabel, reuse, or mutate any historical suite, evidence bundle, operation, Codex run, ref, workflow, artifact, hash, diagnostic, or available root. In particular, preserve RC.4 suite /tmp/revolvr-ext20-rc4.DGg1pW/suite and operation ext20-2bd21aea4f72-01 byte-for-byte and never use them as candidate or live authority.
- Verify exact clean source commit 19c1ef4b6a610016487880aa8ad69ec0204bd4f7 and tree 2fb39c93694e72d986e7a8a849a542fc1bf1728d, and verify that source commit is published and reachable from origin/main. Later controller-only state and helper commits are not candidate source. The helper agent-ext20-rc5.sh must not be copied into the candidate commit or clean clones.
- First rerun the focused lifecycle-routing authority, supervisor prompt/provenance, cycle fail-closed, Structured Outputs compatibility, production autonomous happy-path, and strict-fake regressions. These are local regression checks only and do not claim live API acceptance.
- Do exactly one bounded task: construct and locally verify collision-free candidate level1-v0.1.0-rc.5 from that exact clean source commit.
- Follow the settled EXT-18 reproducible procedure with exact Go 1.26.5 and release version 0.1.0: verify the Go 1.22.12 source floor, run required tests, vet, module verification, vulnerability checks, build Linux/Darwin/FreeBSD amd64 with the settled environment and flags, verify embedded version/source/tool/target/CGO/vcs.modified authority, and require two independent non-local clean-clone builds to be byte-identical.
- Retain a new immutable RC.5 candidate bundle and separate verification evidence with exact build instructions, source/tree/tool identities, artifact hashes, metadata, inventories, the focused lifecycle-authority and Structured Outputs guards, and production happy-path/strict-fake proof. Check local and remote RC.5 ref, bundle, workflow, artifact, run-root, and diagnostic collisions before construction.
- Do not publish a candidate ref, request remote CI, add an attestation workflow, prepare an EXT-20 suite, run a live model operation, tag a release, approve external use, or mark EXT-20 complete.
- Update durable state, decisions, and handoff with the exact RC.5 source/tree, hashes, bundle paths/inventories, verification, preservation evidence, and the next collision-free raw-Git candidate-ref and remote-CI step. Keep EXT-20 unchecked."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
