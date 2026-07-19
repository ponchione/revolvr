#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.4 replacement candidate:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest RC.3 rejection, failed first schema-repair audit, completed follow-up repair, independent review, and publication state before acting.
- Preserve RC.1, RC.2, and RC.3 as immutable rejected history. Never retry, reconcile, relabel, reuse, or mutate any historical suite, evidence bundle, operation, Codex run, ref, workflow, artifact, hash, or diagnostic. In particular, never use RC.3 suite /tmp/revolvr-ext20-rc3.Qghf19/suite or operation ext20-802d9db69596-01 as candidate or live authority, whether or not that temporary path remains available.
- Verify exact clean source commit 2546913e38ec273f64417dece2f91df78fd42fc2 and tree 8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5, and verify that source commit is published and reachable from origin/main. Later controller-only state and helper commits are not candidate source. The helper agent-ext20-rc4.sh must not be copied into the candidate commit or clean clones.
- First rerun the focused Structured Outputs compatibility guard and the production autonomous happy path. These are local regression checks only and do not claim live API acceptance.
- Do exactly one bounded task: construct and locally verify collision-free candidate level1-v0.1.0-rc.4 from that exact clean source commit.
- Follow the settled EXT-18 reproducible procedure with exact Go 1.26.5 and release version 0.1.0: verify the Go 1.22.12 source floor, run required tests, vet, module verification, vulnerability checks, build Linux/Darwin/FreeBSD amd64 with the settled environment and flags, verify embedded version/source/tool/target/CGO/vcs.modified authority, and require two independent non-local clean-clone builds to be byte-identical.
- Retain a new immutable RC.4 candidate bundle and separate verification evidence with exact build instructions, source/tree/tool identities, artifact hashes, metadata, inventories, the focused Structured Outputs guard, and production happy-path proof. Check local and remote RC.4 ref, bundle, workflow, artifact, run-root, and diagnostic collisions before construction.
- Do not publish a candidate ref, request remote CI, add an attestation workflow, prepare an EXT-20 suite, run a live model operation, tag a release, approve external use, or mark EXT-20 complete.
- Update durable state, decisions, and handoff with the exact RC.4 source/tree, hashes, bundle paths/inventories, verification, preservation evidence, and the next collision-free raw-Git candidate-ref and remote-CI step. Keep EXT-20 unchecked."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
