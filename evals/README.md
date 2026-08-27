# Architecture 022 deterministic evaluation

The source-of-truth entrypoint is:

```bash
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/evaluation -count=2
```

It loads `scenarios.json`, creates a fresh local Git repository from
`fixtures/repository` for each of the 20 Section 23.1 scenarios, and compares
the complete canonical result with `golden/baseline.json`. All clocks, IDs,
model outcomes, embedding outcomes, sandbox outcomes, and effect failures are
deterministic. Git uses a replacement environment, fixed identity/time, local
objects only, disabled hooks, and no operator home. The suite invokes no live
model, public network service, daemon, queue, notification, runtime package
install, or ambient credential.

`direct_tools_v1` is the only admitted mode. The reserved
`programmatic_workspace_v1` value returns the typed
`not_implemented_not_admitted` refusal before fixture source, model, sandbox,
or acceptance effects. It is retained only as a historical compatibility
fixture, is never substituted with direct-tool behavior, and has no current
implementation roadmap under ADR-025.

The golden records measured deterministic-fixture baselines. Token fields are
null with explicit omissions because the fakes report no model usage. Logical
wall time comes from the scripted monotonic fixture clock. No cost, token, or
quality threshold is estimated. Retrieval quality fixtures are in
`retrieval/queries.json`; exact-source priority, the selected Architecture 021
Qwen identity, stale-index behavior, missing-embedding degradation, and
context-manifest byte identity are checked by production retrieval/context
code.

Golden changes are review authority, not worker output. Regenerate them only
for an intentional Architecture 022 contract change, inspect the complete
diff, and explain the changed fixture or implementation identity:

```bash
REVOLVR_UPDATE_EVALUATION_GOLDEN=1 go test ./internal/evaluation -run TestGoldenBaseline -count=1
git diff -- evals/golden/baseline.json
```

Live dogfood is separate and destructive to local Revolvr runtime state. It is
never run by ordinary tests:

```bash
./scripts/dogfood-live.sh
```

A live record must name the exact source commit, model policy, prompt versions,
sandbox profile, task contract, and typed outcome. Live evidence does not
replace this deterministic baseline.
