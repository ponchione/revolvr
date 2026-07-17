#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for EXT-20 replacement candidate remediation:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest EXT-20 candidate-rejection state before acting. RC.1 at ed65049fba6bf82852fd406ebc17afa90a953e3f is immutable rejected evidence and must not be changed, overwritten, relabeled, or reused as the replacement.
- Verify the production CLI omitted-work-directory fix and its regression tests first.
- Do exactly one bounded task: construct and locally verify a collision-free Level-1 replacement candidate labeled level1-v0.1.0-rc.2 from the exact clean current source commit.
- Follow the settled EXT-18 reproducible procedure with Go 1.26.5: run the required tests, vet, module verification, vulnerability check, supported Linux/Darwin/FreeBSD amd64 builds, embedded version/source checks, and two independent clean-directory builds with exact hash comparison.
- Retain a new immutable bundle with build instructions, source/tool identities, artifact hashes, and verification evidence. Do not mutate any RC.1 bundle or failed EXT-20 root.
- Do not push a candidate branch, request remote CI, start EXT-20 live operations, tag a release, or mark EXT-20 complete in this pass.
- Update durable state with the new source commit, hashes, verification results, bundle path, and the exact remaining remote-attestation step."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
