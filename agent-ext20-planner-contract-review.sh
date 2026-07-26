#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 planner-contract remediation review:
- Never use gh. Do not commit or push; the controller and operator retain publication authority.
- Do exactly one bounded task: independently review the current uncommitted planner-contract remediation and its tests. Do not implement another source fix unless the reviewed change has a concrete correctness defect; if one is found, record it as a blocker and stop.
- Do not create or run a release candidate, suite, Revolvr operation, live confirmation path, nested Codex/model operation, tag, release, external-use decision, or queue authority. Keep EXT-20 unchecked.
- Treat RC.6 and RC.7 as immutable terminal failed-attempt evidence. Never execute, retry, repair, delete, mutate, derive from, or reuse any RC.6/RC.7 wrapper, launcher, suite, operation, launch record, or evidence root.
- First reverify the RC.7 terminal bundle read-only with: (cd .revolvr/ext20-rc7.rpIUM5/suite/evidence/repo-a/01-successful-source-change-1 && sha256sum -c files.sha256 && sha256sum -c bundle.sha256). Repeat that verification after all review commands.
- Inspect the retained RC.7 planner output and prompt, then inspect the complete current path in .agent/profiles/planner.md, internal/prompt/profile.go, internal/autonomouscycle/worker_prompt.go, internal/autonomousplanning/schema.go, internal/autonomousplanning/contracts.go, and internal/autonomous/state.go plus all changed focused tests.
- Require one coherent contract: pending/in_progress plan steps use evidence [] and rationale null; completed steps require terminal evidence and null rationale; skipped steps require rationale. Pending acceptance criteria use empty evidence and null rationale. Grounding evidence belongs in plan provenance or top-level inputs. Do not weaken PlanStep.Validate or AcceptanceCriterion.Validate.
- Verify that nested anyOf schema branches are in the supported Structured Outputs subset, every object remains closed with every property required, status branches are distinct, and the schema represents every Go-valid lifecycle disposition. Verify that JSON-required empty arrays compare semantically unchanged with omitted empty slices after canonical state round-trips without weakening nonempty terminal evidence comparison.
- The expected source/test scope is .agent/profiles/planner.md; internal/prompt/profile.go and profile_test.go; internal/autonomouscycle/worker_prompt.go and cycle_test.go; and internal/autonomousplanning/schema.go, contracts.go, and contracts_test.go. Preserve unrelated pre-existing .agent state changes and this review launcher.
- Run bash -n agent-ext20-planner-contract-review.sh; gofmt inspection for every changed Go file; go test -count=1 ./internal/autonomousplanning ./internal/prompt ./internal/autonomouscycle; the same focused packages with -race; go test -count=1 ./internal/app -run '^(TestProductionAutonomousHappyPath|TestStrictFakeCodexContract)$'; go test -count=1 ./...; and git diff --check.
- If and only if the review passes, update only .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with the independent result and the next controller publication decision. Keep the handoff concise and do not create another continuation launcher. Stop after this review."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
