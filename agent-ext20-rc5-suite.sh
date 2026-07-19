#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 no-model suite preparation:
- Never use gh. Git operations must use raw git; GitHub evidence must use read-only public REST.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass. Do not pass --live or the live confirmation value.
- Read the newest RC.5 local-candidate, exact-candidate remote-CI, and remote artifact-attestation state before acting.
- Preserve RC.1 through RC.4 as immutable rejected history. Never retry, reuse, recreate, reconcile, relabel, or mutate any historical suite, operation, evidence, workflow, ref, bundle, artifact, hash, run, diagnostic, or sentinel. In particular, preserve terminal RC.4 suite /tmp/revolvr-ext20-rc4.DGg1pW/suite and operation ext20-2bd21aea4f72-01 byte-for-byte.
- Before editing, reverify exact RC.5 candidate ref refs/heads/level1-v0.1.0-rc.5 at 19c1ef4b6a610016487880aa8ad69ec0204bd4f7; exact attestation ref refs/heads/level1-v0.1.0-rc.5-attestation and workflow/main commit 109b38cdb309b50c38ab2ef0df33998e92dfd5e6; workflow SHA-256 9c650a1fbbad1354cf7e991018bb505aba59698c8fec4bc828260c512b069852; successful candidate CI run 29697069305; successful attestation run 29698647782 and job 88223716039; artifact 8445792045 named level1-v0.1.0-rc.5-attestation with digest sha256:ab0febbc035f634d39babb897edd0c94bfaf1805ebc212e767a551fb1758b0e2; and successful companion ten-job CI run 29698647807.
- Do exactly one bounded task: update the guarded external Level-1 suite from terminal RC.4 to immutable RC.5 authority, verify it without model calls, and prepare one new collision-free no-model suite root for the separately confirmed live pass.
- In scripts/dogfood-external-level1-suite.sh bind exact source 19c1ef4b6a610016487880aa8ad69ec0204bd4f7, Linux SHA-256 1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043, and bundle .revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61. Preserve release output revolvr 0.1.0 and exact Codex 0.144.4 authority. Make no unrelated plan, schema, scenario, threshold, configuration, confirmation, or collector change.
- Verify shell syntax, the complete sealed RC.5 candidate and verification bundles, suite --static, and collector fixture/manifest behavior. Run go test -count=1 ./... and git diff --check for the tracked shell-only change.
- Create a new parent with mktemp -d using an RC.5-specific /tmp/revolvr-ext20-rc5.XXXXXX template, then run --prepare --run-root <parent>/suite --install-codex. Preparation may install the exact package but must start no model.
- Independently inspect the prepared authority, candidate and Codex identities, effective 32-minute source-lock authority, zero operation manifests, empty aggregate, both clean disposable repositories, exact task readiness, and refusal of live mode without confirmation without mutating the prepared suite.
- Retain and report the exact prepared suite path and exact confirmation-gated live command, but do not execute it.
- Keep EXT-20 unchecked. Update durable state, decisions, and handoff with files, verification, prepared-root evidence, and the remaining separately confirmed live step. This pass grants no live-model call, tag, release, external-use approval, or EXT-20 completion."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
