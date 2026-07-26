#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
VALIDATION_ROOT="/tmp/revolvr-builder-validation.maYqgv"
DRAFT="$VALIDATION_ROOT/candidate-construction-draft.sh"
EVIDENCE_MANIFEST="$VALIDATION_ROOT/evidence-manifest.tsv"
DRAFT_SHA="71b196d77b6eb89157492609b89e51d0c56a4e418b10b0f28ed43c94d5a4210d"
EVIDENCE_MANIFEST_SHA="2ae2f598dffd37f333c672f492438a81fb346c896964c0107d063423a515ae85"
SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"

cd "$ROOT"
[[ -f .agent/LOOP_PROMPT.md ]] || {
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
}
[[ -d "$VALIDATION_ROOT" && ! -L "$VALIDATION_ROOT" ]] || {
	printf 'Neutral validation root is absent or unsafe\n' >&2
	exit 1
}
[[ "$(stat -c '%a' "$VALIDATION_ROOT")" == "500" ]] || {
	printf 'Neutral validation root mode changed\n' >&2
	exit 1
}
for path in "$DRAFT" "$EVIDENCE_MANIFEST"; do
	[[ -f "$path" && ! -L "$path" && "$(stat -c '%a' "$path")" == "444" ]] || {
		printf 'Sealed validation file changed: %s\n' "$path" >&2
		exit 1
	}
done
[[ "$(stat -c '%s' "$DRAFT")" == "24352" && "$(wc -l <"$DRAFT")" == "575" ]] || {
	printf 'Neutral draft metadata changed\n' >&2
	exit 1
}
[[ "$(sha256sum "$DRAFT" | awk '{print $1}')" == "$DRAFT_SHA" ]] || {
	printf 'Neutral draft hash changed\n' >&2
	exit 1
}
[[ "$(stat -c '%s' "$EVIDENCE_MANIFEST")" == "3695" && "$(wc -l <"$EVIDENCE_MANIFEST")" == "38" ]] || {
	printf 'Evidence-manifest metadata changed\n' >&2
	exit 1
}
[[ "$(sha256sum "$EVIDENCE_MANIFEST" | awk '{print $1}')" == "$EVIDENCE_MANIFEST_SHA" ]] || {
	printf 'Evidence-manifest hash changed\n' >&2
	exit 1
}
while IFS=$'\t' read -r mode size sha name; do
	path="$VALIDATION_ROOT/$name"
	[[ "$name" != */* && -f "$path" && ! -L "$path" ]] || {
		printf 'Unsafe evidence entry: %s\n' "$name" >&2
		exit 1
	}
	[[ "$(stat -c '%a' "$path")" == "$mode" ]] || {
		printf 'Evidence mode changed: %s\n' "$name" >&2
		exit 1
	}
	[[ "$(stat -c '%s' "$path")" == "$size" ]] || {
		printf 'Evidence size changed: %s\n' "$name" >&2
		exit 1
	}
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$sha" ]] || {
		printf 'Evidence hash changed: %s\n' "$name" >&2
		exit 1
	}
done <"$EVIDENCE_MANIFEST"
[[ "$(find "$VALIDATION_ROOT" -maxdepth 1 -type f | wc -l)" == "40" ]] || {
	printf 'Neutral validation file count changed\n' >&2
	exit 1
}
if rg -n -i 'rc12|rc\.12|level1-v0\.1\.0-rc\.12' "$DRAFT"; then
	printf 'Forbidden literal found in neutral draft\n' >&2
	exit 1
fi
[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || {
	printf 'Published source tree changed\n' >&2
	exit 1
}
git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD || {
	printf 'Published source commit is no longer in controller history\n' >&2
	exit 1
}
git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum || {
	printf 'Product source differs from the published source boundary\n' >&2
	exit 1
}
git diff --check

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 neutral-builder independent review pass:
- Do exactly one bounded task: independently review the sealed neutral draft and evidence at $VALIDATION_ROOT. Do not construct, publish, copy, repair, execute, or derive a candidate builder from the draft.
- Never use gh. Use raw Git only for read-only Git checks. Do not commit, push, fetch, create refs, workflows, tags, artifacts, bundles, suites, launch records, releases, or external-use actions.
- Do not start Revolvr, another nested Codex/model operation, a live task, product tests/builds, candidate full mode, or neutral admission. Bash syntax parsing and read-only evidence/source/history/tool/collision inspection are the only draft checks allowed.
- Treat every historical object and the complete sealed neutral root as immutable. Verify the exact root, draft, manifest, evidence identities, before/after preservation comparison, probe semantics, static audit, and expected full-mode refusal. Reprove the absence of every prospective candidate runtime identity, allowing only the validation-pass launcher and this review launcher.
- Keep EXT-20 unchecked. Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with the independent review result. Do not create a construction launcher; any later construction requires separate explicit operator authority. Stop after the review."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
