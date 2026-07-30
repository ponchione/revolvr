#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

[[ -f .agent/LOOP_PROMPT.md ]] || {
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for one bounded independent review of the EXT-20 RC.12 volatile-root recovery preparation:
- Never use gh. Use raw Git for reads. Do not commit or push. Do not start product tests/builds, a builder, construction, candidate, full mode, suite, Revolvr operation, or any additional nested Codex/model process.
- Review only the current uncommitted recovery preparation. Expected scope is .agent/HANDOFF.md, .agent/STATE.md, .agent/DECISIONS.md, .agent/TASKS.md, new agent-ext20-rc12-builder-revalidation-v4.sh, and this review launcher. Reject unrelated tracked, staged, or untracked changes.
- Independently reproduce that /tmp/revolvr-builder-revalidation-v3.PKfbRl is absent, the published v3 review launcher exits at its line-28 root guard before codex exec, local/fetched/public main remain exact, RC.12 runtime/ref/tag/workflow/candidate identities remain absent, and RC.12 is unconsumed.
- Review agent-ext20-rc12-builder-revalidation-v4.sh line by line. It must require a clean published controller before execution, treat all lost /tmp roots as terminal instead of reconstructing them, create fourth anonymous evidence only under persistent ignored .revolvr state, retain the two-sequence neutral gate, address all four recorded full-design concerns, create no candidate authority, and provide explicit diagnostics for its guards.
- Confirm the fourth-pass prompt cannot use old transcripts or prior draft bytes, cannot put authoritative evidence only in /tmp, cannot require lost roots as full-mode inputs, and cannot run product/full construction. Confirm its only successful continuation is an inert persistent-root review launcher and that EXT-20 remains unchecked.
- Run bash -n on both new launchers, inspect their modes, run git diff --check, and verify the durable-state wording and exact next-gate command agree. Do not run either launcher's no-argument mode. The v4 launcher's --preflight-only is expected to refuse while the controller tree is uncommitted and dirty.
- If review passes, make no change and report the exact accepted scope plus the remaining explicit commit/push authorization. If a blocking defect exists, make only the smallest recovery-scope repair, update durable state, rerun relevant checks, and stop."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
