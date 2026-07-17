#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for EXT-20:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will verify, commit, and push.
- Do not start live or nested Codex operations in this implementation pass.
- Preserve candidate source commit ed65049fba6bf82852fd406ebc17afa90a953e3f and its recorded artifact hashes.
- Add the smallest guarded shell driver for the complete EXT-20 Level-1 dogfood suite.
- The driver must require an unmistakable explicit live-run confirmation flag before any model call.
- It must install or accept an isolated @openai/codex@0.144.4 package, then verify exact version codex-cli 0.144.4 and SHA-256 134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477.
- It must verify and use the exact EXT-18 Linux candidate, create two new disposable external Git repositories plus outside sentinels and evidence roots, and invoke scripts/dogfood-external-level1.sh once per unique operation.
- Cover at least ten real-Codex operations: five successful source changes plus verification failure/correction, needs input, graceful cancellation/restart, and safety refusal. Never edit runtime evidence to manufacture an outcome.
- Make reruns collision-safe, retain every terminal bundle, verify every manifest, and produce a deterministic aggregate report that fails unless all EXT-20 quantitative and zero-tolerance conditions pass.
- Provide a no-model static or preparation mode for local verification.
- Keep EXT-20 unchecked until the real suite has run and every retained manifest passes independent validation.
- Update durable state with implementation, verification, remaining live-run command, and blockers."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
