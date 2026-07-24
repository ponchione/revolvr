#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.5 live-failure remediation requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.5 live-failure remediation requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 terminal live-failure remediation:
- Never use gh. Use raw Git for Git reads. Do not commit or push; the controller will independently review and publish later.
- Do not start any Revolvr live operation or nested Codex/model operation. Never pass the EXT-20 live confirmation token to any command. Do not prepare or name a new release candidate in this pass.
- Do exactly one bounded task: fix the product and retained-receipt root causes proven by the first RC.5 live operation, permanently retire its direct live path, and add focused regressions. Keep EXT-20 unchecked.
- Treat RC.5 source 19c1ef4b6a610016487880aa8ad69ec0204bd4f7 and suite ext20-9b7cc650ea0c as immutable rejected history. Never retry, resume, reconcile, relabel, mutate, or reuse the suite or operation.
- Before editing, independently verify and inspect launch record /home/gernsback/source/revolvr/.revolvr/ext20-rc5-launch-records/ext20-9b7cc650ea0c-20260724T230449Z-468858 and evidence /home/gernsback/source/revolvr/.revolvr/ext20-rc5-no-start-replacement.PNKJ20/suite/evidence/repo-a/01-successful-source-change-1. Preserve the full suite and launch record byte-for-byte. Initial path-bearing content-stream SHA-256 values are suite 875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c, evidence 9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76, and launch record f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8.
- Require scripts/dogfood-external-level1.sh --verify-manifest to pass for that evidence. Bind the exact operation result: scenario successful-source-change-1 expected completed, observed unsafe_or_ambiguous, run exit 1, one plan attempt, zero source changes/verification/audits/corrections/commits, and stop detail cycle dossier identity is missing or malformed. Do not reinterpret launcher status 130 as the suite's terminal product outcome; retain both facts.
- Fix internal/autonomousplanapply at the root: a valid planner role-projected dossier uses schema autonomous-role-dossier-manifest-v1, but the planning application gate currently accepts only the legacy autonomous-task-dossier-manifest-v1 schema. Accept only the exact appropriate legacy or planner-role authority, preserve every task/hash/size/source/provenance comparison, and fail closed for missing, malformed, wrong-role, or mismatched dossiers. Follow the already role-aware audit-application pattern where appropriate; do not weaken dossier validation or add a dogfood special case.
- Fix fallback receipt identity at the root: a nonblank task body must preserve its exact bytes, including its final newline, instead of being TrimSpace-normalized. Keep blank-value fallback behavior and intentional normalization of identifier/status fields.
- Fix fallback receipt claim consistency at the root: the human no-verification placeholder must not parse as a verification claim. Preserve exact structured verification entries when present. Add regression coverage proving fallback frontmatter/body parsing and ordinary receipt validation agree for an exact task with a final newline and zero verification claims.
- Permanently fail-close agent-ext20-rc5-live-direct.sh as retired terminal authority and update its focused shell check so no argument can enter a live path or alter the retained suite/record. The misleading interrupted launcher record and later child terminal evidence must be documented, but do not create, delete, rewrite, or repair runtime evidence in place. A future candidate may receive a separately designed foreground/visible child boundary only after independent review.
- Add focused tests for both accepted planning dossier forms, the exact planner role, wrong-role/malformed refusal, exact fallback task bytes, empty verification claims, and the evidence-shaped regression. Preserve existing style and architecture; add no dependency and make no unrelated candidate, workflow, suite, task, release, or configuration change.
- Format changed Go files with gofmt. Run focused ordinary and race tests for every changed Go package, the production autonomous happy path and relevant strict-fake path, scripts/check-ext20-rc5-live-direct.sh, go test -count=1 ./..., git diff --check, manifest verification, and exact suite/evidence/launch-record preservation before and after work.
- Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with files, exact tests, retained failure authority, and the remaining independent review/publication gate. Keep tag, release, external-use approval, RC.6 construction, and EXT-20 completion separate."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
