#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 source-lock configuration defect:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass.
- Preserve RC.1, RC.2, all candidate/attestation refs and bundles, both earlier retired roots, and failed RC.2 suite root /tmp/revolvr-ext20-rc2.96ibla/suite byte-for-byte and metadata-for-metadata unchanged.
- Read the retained terminal evidence for operation ext20-3601e63c616b-01 before changing code. It stopped before any model attempt because config check and doctor admitted the suite's codex timeout of 30m with the default source-writer lock timeout of 5m, while autonomouscycle requires at least CodexTimeout + 2*GitTimeout + 1m = 32m.
- Do exactly one bounded task: fix the root configuration-normalization/preflight inconsistency so every effective run configuration admitted by config check/doctor satisfies the same source-writer lock window required by supervisor/autonomouscycle. Do not work around it by shortening the suite timeout or editing retained external repositories.
- Keep the smallest architecture-consistent behavior: derive a safe default from the finalized Codex and Git timeouts, preserve explicit valid authority, fail closed on invalid/overflowing authority, and derive heartbeat authority only after the final timeout. Update effective-config schema authority if the contract change requires it.
- Add focused regressions for the 30m Codex/30s Git case and relevant custom/invalid cases. Prove config check/doctor no longer report ready authority that execution rejects, without invoking Codex.
- Run gofmt on changed Go files, focused ordinary and race tests, go test -count=1 ./..., relevant CLI config-check/doctor no-model probes, and git diff --check.
- Record the retained failure, zero model attempts, root cause, files, verification, release consequence, and remaining replacement-candidate work in durable state. Keep EXT-20 unchecked. Do not build or publish RC.3 in this pass."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
