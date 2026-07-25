#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.6 remote gate requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.6 remote gate requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.6 candidate-ref and remote-CI gate:
- Never use gh. Use raw Git for Git operations and read-only GitHub REST responses for Actions evidence. Do not commit or push main; the controller will independently review and publish durable-state changes afterward.
- Execution of this launcher is explicit authority for exactly one external mutation: collision-safe creation of refs/heads/level1-v0.1.0-rc.6 at exact source commit 73f1f81f1c51d927114f19818a18161d0fcb8541. Do not create, update, or delete any other ref.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not add or publish an attestation workflow/ref, prepare a suite, use any historical live confirmation token, tag, release, approve external use, grant queue authority, or complete EXT-20.
- Require published local-candidate record commit 60a19d4e38ddd5ea76490221973601e0efce2625 to be an ancestor of exact local/remote main. Read its newest RC.6 local-candidate and independent-review authority before acting.
- Independently reverify the complete sealed RC.6 candidate and verification bundles without changing them. Require candidate ID level1-v0.1.0-rc.6, release 0.1.0, source commit 73f1f81f1c51d927114f19818a18161d0fcb8541, source tree 7c9753461a08b25915f4f53533d91e57d40a20ca, candidate inventory/seal 30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1 / d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88, verification inventory/seal 9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf / f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f, build-instructions SHA-256 94d291ec80db7427bddc1db57cac147c5d061ca3dbdbdd038259e1da3505a906, and Linux/Darwin/FreeBSD artifact SHA-256 values f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d, 596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7, and 60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344.
- Treat RC.1 through RC.5 and all historical refs, workflows, bundles, artifacts, suites, operations, launch records, diagnostics, and retained evidence as immutable rejected history. Preserve the RC.5 suite/evidence/launch-record content-stream SHA-256 values 875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c, 9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76, and f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8 before and after work. Require agent-ext20-rc5-live-direct.sh to remain permanently failed closed.
- Do exactly one bounded task: publish the exact RC.6 candidate ref and require the complete push-triggered EXT-15 CI matrix to finish successfully on that exact source SHA.
- Immediately before publication, fetch origin without tags; prove the source is published and reachable from origin/main; prove refs/heads/level1-v0.1.0-rc.6 is absent locally and remotely; and prove no RC.6 tag, attestation ref, workflow, or remote artifact-name collision exists. Fail closed on any collision, ambiguity, or identity drift.
- Create only the candidate ref with raw Git using an empty-expected force-with-lease: git push --force-with-lease=refs/heads/level1-v0.1.0-rc.6: origin 73f1f81f1c51d927114f19818a18161d0fcb8541:refs/heads/level1-v0.1.0-rc.6. Read it back and require the exact SHA. Never force-update, delete, or move an existing ref.
- Locate the new push-triggered GitHub Actions CI run through the public REST API. Require event push, head_branch level1-v0.1.0-rc.6, and head_sha 73f1f81f1c51d927114f19818a18161d0fcb8541, then poll with a finite bound until completion. Fail if the exact run cannot be identified unambiguously or does not conclude success.
- Require exactly the ten mandatory successful jobs for that run: Go 1.22 source floor and tests; Production autonomous strict-fake suite; Race tests; Vet and module verification; Fake-Codex success smoke; Fake-Codex verification-failure smoke; Build linux/amd64; Build darwin/amd64; Build freebsd/amd64; and Build Windows diagnostic stub. Record exact run and job IDs, URLs, head SHA, status, and conclusions in durable state.
- Stop after remote CI evidence. Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with exact ref readback, CI run/job evidence, preservation checks, and the next separately bounded exact-checkout Go 1.26.5 artifact-attestation workflow gate. Keep .agent/TASKS.md unchanged with EXT-20 unchecked."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
