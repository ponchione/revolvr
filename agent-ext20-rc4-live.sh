#!/usr/bin/env bash
set -euo pipefail

readonly LIVE_CONFIRMATION="EXT20_LIVE_REAL_CODEX_MODEL_CALLS"

if [[ "$#" -ne 1 || "$1" != "$LIVE_CONFIRMATION" ]]; then
	printf 'usage: %s %s\n' "${0##*/}" "$LIVE_CONFIRMATION" >&2
	exit 64
fi

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the separately confirmed EXT-20 RC.4 live suite:
- The operator supplied the exact confirmation token EXT20_LIVE_REAL_CODEX_MODEL_CALLS to this launcher. This grants actual real-Codex/model calls only through the exact retained RC.4 suite described below.
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push after the pass.
- Do exactly one bounded task: reverify the retained prepared suite without mutation, execute its exact live command once, then independently verify all retained manifests and the aggregate. Do not prepare another suite or run any operation separately.
- Exact prepared root: /tmp/revolvr-ext20-rc4.DGg1pW/suite. Require suite ID ext20-2bd21aea4f72, authority SHA-256 4f9b653c9e62e5fc5932b219952bbe61fccd79d331ac2bd7fcf2c570035eacb7, operation-plan SHA-256 5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1, content fingerprint 5e988363634a5aa4739c3b4bfccce865d2cf6e2c7ddb634aaa4eb25750641305, and metadata/layout fingerprint 5e52e1be955403644fd33ee2b95c832896994305f95806ebac533ec93525244f before live execution.
- Require exact candidate source 2546913e38ec273f64417dece2f91df78fd42fc2, Linux SHA-256 98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe, release output revolvr 0.1.0, exact Codex 0.144.4 version/hash authority, both clean repository heads recorded in durable state, effective 32-minute source-writer authority, all ten tasks ready, and zero operation/collector manifests plus an empty aggregate. Any pre-live drift fails closed without model calls.
- Preserve RC.1, RC.2, and RC.3 as immutable rejected history and preserve all RC.4 candidate, CI, attestation, workflow, bundle, hash, and prepared-root authority. Never retry, reconcile, relabel, or mutate a historical failed suite or operation.
- After the complete pre-live gate passes, execute exactly once: scripts/dogfood-external-level1-suite.sh --live --run-root /tmp/revolvr-ext20-rc4.DGg1pW/suite --confirm-live-real-codex EXT20_LIVE_REAL_CODEX_MODEL_CALLS
- Let the guarded suite complete normally. Do not manufacture, edit, omit, or reinterpret runtime evidence. If it fails or is interrupted, preserve every terminal artifact, record the exact failure, leave EXT-20 unchecked, and stop without retrying.
- On suite success, run --verify-suite against the exact root and independently verify every operation and collector manifest, repository/sentinel authority, aggregate identity, plan coverage, outcomes, quantitative thresholds, and zero-tolerance conditions. Mark EXT-20 complete only if every acceptance condition passes from retained evidence.
- Update durable state, decisions, tasks, and handoff with exact operations, Codex run identities, verification, aggregate, preservation, result, and remaining work. Do not tag, release, or approve external use in this pass."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
