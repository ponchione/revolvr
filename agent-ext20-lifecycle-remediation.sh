#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.4 lifecycle-authority remediation:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass.
- Read and independently verify the terminal RC.4 evidence at /tmp/revolvr-ext20-rc4.DGg1pW/suite/evidence/repo-a/01-successful-source-change-1 before editing. Preserve the entire RC.4 suite, evidence, candidate, refs, workflow, artifact records, diagnostics, hashes, and runs unchanged. Never retry, reconcile, relabel, or reuse RC.4 or its failed operation.
- Treat RC.4 as immutable rejected history. Operation ext20-2bd21aea4f72-01 expected completed but terminated unsafe_or_ambiguous after one supervisor decision and zero worker attempts because a pending task's supervisor prompt admitted the general action vocabulary without communicating that pending lifecycle allows only plan, block, or needs_input. The model selected implement, and the unchanged lifecycle policy correctly rejected it.
- Do exactly one bounded task: fix the root prompt/authority mismatch so each supervisor decision receives exact current lifecycle routing authority before choosing an action. Preserve the lifecycle gate; do not allow implement directly from pending and do not add fallback, automatic coercion, hidden retry, or special-case dogfood behavior.
- Prefer one deterministic source of truth shared with policy enforcement so prompt-facing admitted actions cannot drift from runtime lifecycle admission. Fail closed for lifecycle states that admit no new routing.
- Add focused regression coverage for the exact RC.4 condition: pending lifecycle with no plan must expose only plan, block, and needs_input as legal next actions and must not expose implement as admitted. Cover all lifecycle mappings needed to prove prompt/policy agreement and deterministic prompt/provenance output.
- Preserve all Structured Outputs strictness, action/profile contracts, evidence validation, source locks, attempts, verification, audit, and commit authority. Make no unrelated suite, task, candidate, dependency, configuration, or release change.
- Format changed Go files with gofmt. Run focused ordinary and race tests for every changed package, the production autonomous happy path and relevant strict-fake path, go test -count=1 ./..., and git diff --check.
- Keep EXT-20 unchecked. Update durable state, decisions, and handoff with exact failure authority, files, tests, remaining independent review/commit gate, and later collision-free RC.5 construction prerequisites. Do not construct RC.5 in this pass."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
