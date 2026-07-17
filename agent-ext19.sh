#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for EXT-19:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will verify, commit, and push.
- Preserve candidate source commit ed65049fba6bf82852fd406ebc17afa90a953e3f.
- Add the smallest supplemental GitHub Actions attestation workflow, triggered by a push to level1-v0.1.0-rc.1-attestation.
- It must explicitly check out the exact candidate commit, install Go 1.26.5, reproduce the exact EXT-18 Linux/Darwin/FreeBSD amd64 build commands and version metadata, compare each SHA-256 with the recorded EXT-18 hash, and retain the binaries plus hash manifest as workflow artifacts.
- Keep EXT-19 unchecked until that remote workflow passes.
- Update durable state with the implementation and local verification."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
