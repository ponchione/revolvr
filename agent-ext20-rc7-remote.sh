#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.7 remote gate requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.7 remote gate requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.7 candidate-ref and remote-CI gate:
- Never use gh. Use raw Git for Git operations and read-only GitHub REST responses for Actions evidence. Do not commit or push main; the controller will independently review and publish durable-state changes afterward.
- Execution of this launcher is explicit authority for exactly one external mutation: collision-safe creation of refs/heads/level1-v0.1.0-rc.7 at exact source commit f63cbe3989cb281652cf4eec3f92614fec98294d. Do not create, update, or delete any other ref.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not add or publish an attestation workflow/ref, prepare a suite, use any live confirmation token, tag, release, approve external use, grant queue authority, or complete EXT-20.
- Require published local-candidate record commit 1055d7fc8ccdb844b5fe1405674133a244a1be64 to be an ancestor of exact local/remote main. Read its newest RC.7 local-candidate and independent-review authority before acting.
- Independently reverify the complete sealed RC.7 candidate and verification bundles without changing them. Require candidate ID level1-v0.1.0-rc.7, release 0.1.0, source commit f63cbe3989cb281652cf4eec3f92614fec98294d, source tree 43fc099d966cd6c06a74f00272c945fe3ca0a0f9, candidate inventory/seal 7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba / 2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0, verification inventory/seal ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733 / 6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b, build-instructions SHA-256 ccf6cba57b00b3bdf1d50b074e4bbe9f13e3579493c22e87682f9d5952048ecd, and Linux/Darwin/FreeBSD artifact SHA-256 values 1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a, fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086, and 13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe.
- Treat RC.1 through RC.6 and all historical refs, workflows, bundles, artifacts, suites, operations, launch records, diagnostics, and retained evidence as immutable rejected or failed history. Verify the RC.6 terminal manifest and preserve the RC.6 suite, launch-record, and terminal-evidence content-stream SHA-256 values d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b, 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce, and e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259 before and after work. Never execute or mutate any RC.6 launcher, suite, or evidence.
- Do exactly one bounded task: publish the exact RC.7 candidate ref and require the complete push-triggered EXT-15 CI matrix to finish successfully on that exact source SHA.
- Immediately before publication, fetch origin without tags; prove the source is published and reachable from origin/main; prove refs/heads/level1-v0.1.0-rc.7 is absent locally and remotely; and prove no RC.7 tag, attestation ref, workflow, or remote artifact-name collision exists. Fail closed on any collision, ambiguity, or identity drift.
- Create only the candidate ref with raw Git using an empty-expected force-with-lease: git push --force-with-lease=refs/heads/level1-v0.1.0-rc.7: origin f63cbe3989cb281652cf4eec3f92614fec98294d:refs/heads/level1-v0.1.0-rc.7. Read it back and require the exact SHA. Never force-update, delete, or move an existing ref.
- Locate the new push-triggered GitHub Actions CI run through the public REST API. Require event push, head_branch level1-v0.1.0-rc.7, and head_sha f63cbe3989cb281652cf4eec3f92614fec98294d, then poll with a finite bound until completion. Fail if the exact run cannot be identified unambiguously or does not conclude success.
- Require exactly the ten mandatory successful jobs for that run: Go 1.22 source floor and tests; Production autonomous strict-fake suite; Race tests; Vet and module verification; Fake-Codex success smoke; Fake-Codex verification-failure smoke; Build linux/amd64; Build darwin/amd64; Build freebsd/amd64; and Build Windows diagnostic stub. Record exact run and job IDs, URLs, head SHA, status, and conclusions in durable state.
- Stop after remote CI evidence. Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with exact ref readback, CI run/job evidence, preservation checks, and the next separately bounded exact-checkout Go 1.26.5 artifact-attestation workflow gate. Keep .agent/TASKS.md unchanged with EXT-20 unchecked."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
