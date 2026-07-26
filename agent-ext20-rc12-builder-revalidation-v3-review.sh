#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

PREFLIGHT_ONLY=0
if [[ "$#" == 1 && "$1" == --preflight-only ]]; then
	PREFLIGHT_ONLY=1
elif [[ "$#" != 0 ]]; then
	printf 'Usage: %s [--preflight-only]\n' "$0" >&2
	exit 64
fi

git fetch --no-tags origin main
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'Independent review requires a clean published controller repository\n' >&2
	exit 1
}
HEAD_COMMIT="$(git rev-parse HEAD)"
[[ "$HEAD_COMMIT" == "$(git rev-parse refs/remotes/origin/main)" ]] || exit 1
[[ "$HEAD_COMMIT" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || exit 1
git merge-base --is-ancestor 1507cc80d4b323f71a7635154402c030a94bddc9 HEAD

VALIDATION_ROOT="/tmp/revolvr-builder-revalidation-v3.PKfbRl"
DRAFT="$VALIDATION_ROOT/prospective-builder.sh"
MANIFEST="$VALIDATION_ROOT/evidence-manifest.tsv"
[[ -d "$VALIDATION_ROOT" && ! -L "$VALIDATION_ROOT" ]]
[[ "$(stat -c '%a' "$VALIDATION_ROOT")" == 500 ]]
[[ -f "$DRAFT" && ! -L "$DRAFT" ]]
[[ "$(stat -c '%a:%s' "$DRAFT")" == "444:42446" ]]
[[ "$(wc -l <"$DRAFT")" == 789 ]]
[[ "$(sha256sum "$DRAFT" | awk '{print $1}')" == "302738277108837a7282d9abe5650d34bee5896ec58ef13fefa85b8edf9345fc" ]]
[[ -f "$MANIFEST" && ! -L "$MANIFEST" ]]
[[ "$(stat -c '%a:%s' "$MANIFEST")" == "444:754" ]]
[[ "$(wc -l <"$MANIFEST")" == 8 ]]
[[ "$(sha256sum "$MANIFEST" | awk '{print $1}')" == "5018b79e47ea393eed9a3f0a3646f5d9ae20a65426993ed0bbe5c08a25b7e746" ]]
count=0
while IFS=$'\t' read -r mode size sha name; do
	[[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]
	path="$VALIDATION_ROOT/$name"
	[[ -f "$path" && ! -L "$path" ]]
	[[ "$(stat -c '%a:%s' "$path")" == "$mode:$size" ]]
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$sha" ]]
	count=$((count + 1))
done <"$MANIFEST"
[[ "$count" == 8 ]]
[[ "$(find "$VALIDATION_ROOT" -maxdepth 1 -type f | wc -l)" == 9 ]]
[[ "$(find "$VALIDATION_ROOT" -maxdepth 1 -type f -printf '%s\n' | awk '{sum += $1} END {print sum}')" == 53181 ]]
[[ "$(cd "$VALIDATION_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum | sha256sum | awk '{print $1}')" == "bf01b03207cbd2d3f056c31ee6c001f3efcff02a5fbace30cdeace228473bb77" ]]
[[ -z "$(find -P "$VALIDATION_ROOT" -type l -print -quit)" ]]

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'Third neutral builder independent-review preflight passed\n'
	exit 0
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for one bounded independent review of the third anonymous prospective RC.12 builder-validation record:
- Never use gh. Use raw Git for reads. Do not commit or push. Do not create or execute a builder, construction launcher, candidate, preflight/build/stage/diagnostic root, artifact, bundle, ref, workflow, tag, suite, launch record, Revolvr/model operation, release, or external-use action. Do not run product tests/builds or either draft mode.
- Review only sealed root $VALIDATION_ROOT. Reverify its exact mode, nine-file/53,181-byte inventory, manifest SHA-256 5018b79e47ea393eed9a3f0a3646f5d9ae20a65426993ed0bbe5c08a25b7e746, root stream bf01b03207cbd2d3f056c31ee6c001f3efcff02a5fbace30cdeace228473bb77, and every manifest entry. The draft is 42,446 bytes, 789 lines, SHA-256 302738277108837a7282d9abe5650d34bee5896ec58ef13fefa85b8edf9345fc.
- Treat both recorded neutral validation sequences as evidence to verify, not commands to rerun. Inspect the independently authored draft read-only for correctness: guarded exact-root cleanup restores owner write permission to every directory depth-first, rejects symlinks, deletes depth-first, proves absence, and never hides cleanup failure; success and induced pre-copy failure use two sealed levels and mode-0400 files.
- Verify the semantic copy design writes only under writable parents, seals every file/directory, creates destination separately, uses cp -a source/. destination/., and verifies bytes, complete inventories, modes, single links, distinct inodes, no symlinks, and cleanup. Reject any rename, hard-link, symlink, or broad deletion path.
- Verify the full-context role model requires and hashes the exact builder and separately tracked construction launcher, permits tracked validation-history launchers, treats only true outputs/collisions as forbidden, and requires this third validation root as immutable input after publication. Check complete RC.6-RC.11/prior-root snapshot and checksum preservation plus recorded absences.
- Verify the unexecuted full design uses two non-local shallow exact-commit fetches, detached source a24804bcf2a32ee5434d3686eabad5b72d9f39ba/tree 2c8ee9f6b4283410547a9f99972e25eac06c9e33, clean status, exclusion of later controller objects/launchers, executable build instructions, exact tool/environment/source/controller identity, both Go test/race/vet/module matrices, ordinary/verbose vulnerability results, reproducibility, GOOS/GOARCH/CGO/VCS/build-ID metadata, complete manifests/inventories/hashes, and retained history evidence.
- Verify final publication uses exact mkdir, only then sets appeared state, copies with cp -a, restores stage/root modes, and checks complete stage/final manifests, inventories, hashes, counts, modes, links, distinct inodes, and extra entries. Publication failure must remain terminal without rename/link/symlink fallback or cleanup of an appeared final path.
- Specifically confirm or refute the controller's read-only concern that verification-complete-inventory.tsv is captured before seal_tree changes file/directory modes, then compared with a post-seal final inventory, making full mode deterministically fail after the final verification path appears.
- Compare collision and preservation coverage with the durable prior-root contract. Specifically inspect the exact .revolvr/ext20-rc12 path versus the ext20-rc12.* glob, the omitted RC.8-RC.11 exact runtime roots and retired local/remote ref/tag plus RC.10/RC.11 runtime-glob absences, and whether all candidate/attestation/final-verification Actions artifact names are checked rather than only the candidate name.
- Also verify full-mode controller authority against fetched/public main, signal/error handling after a final path appears, and every claim in the sealed summaries against executable source rather than accepting the summaries as proof.
- Record only the independent review result in .agent/HANDOFF.md, .agent/STATE.md, .agent/DECISIONS.md, and EXT-20 current-gate text. Keep EXT-20 unchecked. If accepted, state that construction still requires a separately published exact builder and construction launcher plus new explicit operator authorization. Create no continuation launcher in this review pass."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
