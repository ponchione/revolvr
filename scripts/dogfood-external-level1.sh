#!/usr/bin/env bash
set -euo pipefail
umask 0022
export LC_ALL=C

SCHEMA_VERSION="revolvr-external-level1-dogfood-manifest-v1"
DISPOSABLE_MARKER=".revolvr-dogfood-disposable-v1"
DISPOSABLE_MARKER_CONTENT="revolvr-external-level1-disposable-v1"
SCRIPT_NAME="dogfood-external-level1"

TMP_ROOT=""
FIXTURE_ROOT=""
FIXTURE_LOCK=""

cleanup() {
  if [[ -n "$TMP_ROOT" && -d "$TMP_ROOT" ]]; then
    rm -rf -- "$TMP_ROOT"
  fi
  if [[ -n "$FIXTURE_ROOT" && "$FIXTURE_ROOT" == "${TMPDIR:-/tmp}/revolvr-external-level1-fixture" && -d "$FIXTURE_ROOT" ]]; then
    rm -rf -- "$FIXTURE_ROOT"
  fi
  if [[ -n "$FIXTURE_LOCK" && "$FIXTURE_LOCK" == "${TMPDIR:-/tmp}/revolvr-external-level1-fixture.lock" && -d "$FIXTURE_LOCK" ]]; then
    rmdir -- "$FIXTURE_LOCK" 2>/dev/null || true
  fi
}
trap cleanup EXIT

fail() {
  echo "$SCRIPT_NAME: $*" >&2
  exit 1
}

usage() {
  cat <<'USAGE'
Usage:
  scripts/dogfood-external-level1.sh --fixture-only --evidence-dir <new-dir>
  scripts/dogfood-external-level1.sh --verify-manifest <manifest-or-dir>
  scripts/dogfood-external-level1.sh [real-operation options]

Real-operation options (all required unless noted):
  --repository <path>                  Clean disposable external Git repository
  --confirm-disposable-repository <p> Exact canonical repository path
  --outside-sentinel <path>            Existing sentinel tree outside repository
  --candidate-binary <path>            Exact release-candidate Revolvr binary
  --candidate-binary-sha256 <sha256>   Expected candidate binary identity
  --candidate-source-commit <oid>      Expected vcs.revision in Go build metadata
  --candidate-version-output <text>    Exact single-line `revolvr --version` output
  --codex-executable <path>             Exact release-listed Codex executable
  --codex-sha256 <sha256>              Expected Codex executable identity
  --codex-version <text>               Exact single-line Codex version
  --approved-config-sha256 <sha256>    Exact .revolvr/config.yaml identity
  --task <task-id>                     Exact reviewed autonomous task
  --operation-id <operation-id>        Stable task operation identity
  --scenario <name>                    Dogfood scenario label
  --expected-outcome <stop-reason>     Expected typed task stop
  --evidence-time <UTC>                Declared RFC3339 UTC evidence/export time
  --evidence-dir <new-dir>             New evidence directory outside repository
  --max-cycles <n>                     Positive finite cycle bound (default 50)

The repository must contain a tracked regular file named
`.revolvr-dogfood-disposable-v1` with exactly this content:
`revolvr-external-level1-disposable-v1`. The explicit confirmation path must
also match. Admission failures create no repository, runtime, sentinel, or
evidence mutation. The collector never edits runtime files to manufacture
recovery evidence.

Fixture-only testing accepts the optional fault injection
`--fixture-fault dirty|non-disposable|wrong-binary|wrong-codex`; each injected
input must be refused before evidence creation and exits nonzero after checking
that the repository and sentinel stayed unchanged.
USAGE
}

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -- "$1" | awk '{print $1}'
  else
    fail "sha256sum or shasum is required"
  fi
}

valid_sha256() {
  [[ "$1" =~ ^[0-9a-f]{64}$ ]]
}

valid_git_oid() {
  [[ "$1" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]]
}

valid_safe_value() {
  [[ -n "$1" && "$1" != *$'\n'* && "$1" != *$'\r'* && "$1" != *$'\t'* ]]
}

canonical_existing_dir() {
  (cd "$1" 2>/dev/null && pwd -P)
}

canonical_existing_file() {
  local value="$1"
  local parent base
  [[ -f "$value" && ! -L "$value" ]] || return 1
  parent="$(canonical_existing_dir "$(dirname -- "$value")")" || return 1
  base="$(basename -- "$value")"
  printf '%s/%s\n' "$parent" "$base"
}

canonical_new_path() {
  local value="$1"
  local parent base
  parent="$(dirname -- "$value")"
  base="$(basename -- "$value")"
  [[ "$base" != "." && "$base" != ".." && -n "$base" ]] || return 1
  parent="$(canonical_existing_dir "$parent")" || return 1
  printf '%s/%s\n' "$parent" "$base"
}

path_is_within() {
  local path="$1"
  local root="$2"
  [[ "$path" == "$root" || "$path" == "$root/"* ]]
}

stat_fields() {
  local path="$1"
  local mode size links modified
  if mode="$(stat -c '%a' -- "$path" 2>/dev/null)"; then
    size="$(stat -c '%s' -- "$path")" || return 1
    links="$(stat -c '%h' -- "$path")" || return 1
    modified="$(stat -c '%Y' -- "$path")" || return 1
  else
    mode="$(stat -f '%Lp' -- "$path")" || return 1
    size="$(stat -f '%z' -- "$path")" || return 1
    links="$(stat -f '%l' -- "$path")" || return 1
    modified="$(stat -f '%m' -- "$path")" || return 1
  fi
  printf '%s\t%s\t%s\t%s\n' "$mode" "$size" "$links" "$modified"
}

regular_file_is_unaliased() {
  local path="$1"
  local links
  [[ -f "$path" && ! -L "$path" ]] || return 1
  if links="$(stat -c '%h' -- "$path" 2>/dev/null)"; then
    :
  else
    links="$(stat -f '%l' -- "$path" 2>/dev/null)" || return 1
  fi
  [[ "$links" == 1 ]]
}

snapshot_tree() {
  local root="$1"
  local output="$2"
  local exclude_git="${3:-false}"
  local paths_file="$TMP_ROOT/tree-paths.$RANDOM"
  : >"$output"
  if [[ ! -e "$root" && ! -L "$root" ]]; then
    printf 'missing\t.\n' >"$output"
    return
  fi
  if [[ "$exclude_git" == true ]]; then
    find "$root" -path "$root/.git" -prune -o -print0 >"$paths_file"
  else
    find "$root" -print0 >"$paths_file"
  fi
  local path rel type fields detail
  local -a paths=()
  while IFS= read -r -d '' path; do
    rel="${path#"$root"}"
    rel="${rel#/}"
    [[ -n "$rel" ]] || rel="."
    if [[ "$rel" == *$'\n'* || "$rel" == *$'\r'* || "$rel" == *$'\t'* ]]; then
      rm -f -- "$paths_file"
      return 1
    fi
    paths+=("$rel")
  done <"$paths_file"
  rm -f -- "$paths_file"
  while IFS= read -r rel; do
    [[ -n "$rel" ]] || continue
    if [[ "$rel" == "." ]]; then
      path="$root"
    else
      path="$root/$rel"
    fi
    fields="$(stat_fields "$path")" || return 1
    detail="-"
    if [[ -L "$path" ]]; then
      type="symlink"
      detail="target=$(printf '%q' "$(readlink -- "$path")")"
    elif [[ -d "$path" ]]; then
      type="directory"
    elif [[ -f "$path" ]]; then
      type="regular"
      detail="sha256=$(hash_file "$path")"
    else
      type="other"
    fi
    printf '%s\t%s\t%s\t%s\n' "$type" "$fields" "$(printf '%q' "$rel")" "$detail" >>"$output"
  done < <(printf '%s\n' "${paths[@]}" | sort)
}

snapshot_git() {
  local repo="$1"
  local dir="$2"
  mkdir -p -- "$dir"
  git -C "$repo" rev-parse --verify HEAD >"$dir/head.txt"
  git -C "$repo" symbolic-ref -q HEAD >"$dir/branch.txt" || : >"$dir/branch.txt"
  git -C "$repo" status --short --branch >"$dir/status-human.txt"
  git -C "$repo" status --porcelain=v1 -z --untracked-files=all >"$dir/status-porcelain-v1-z.bin"
  git -C "$repo" ls-files -s -z >"$dir/index-ls-files-z.bin"
  git -C "$repo" diff --binary --no-ext-diff HEAD >"$dir/worktree.diff"
  git -C "$repo" diff --cached --binary --no-ext-diff HEAD >"$dir/index.diff"
  git -C "$repo" for-each-ref --format='%(refname)%09%(objectname)' | sort >"$dir/refs.tsv"
  git -C "$repo" worktree list --porcelain >"$dir/worktrees.txt"
  local index_path index_fields
  index_path="$(git -C "$repo" rev-parse --git-path index)"
  if [[ "$index_path" != /* ]]; then
    index_path="$repo/$index_path"
  fi
  if [[ -f "$index_path" && ! -L "$index_path" ]]; then
    printf '%s\n' "$(hash_file "$index_path")" >"$dir/index-file.sha256"
    index_fields="$(stat_fields "$index_path")"
    printf '%s\n' "$(printf '%s\n' "$index_fields" | cut -f1-3)" >"$dir/index-file-stat.tsv"
  else
    printf 'missing\n' >"$dir/index-file.sha256"
    printf 'missing\n' >"$dir/index-file-stat.tsv"
  fi
}

compare_git_snapshots() {
  local before="$1"
  local after="$2"
  local diagnostic="$TMP_ROOT/fault-git-authority.diff"
  if diff -r "$before" "$after" >"$diagnostic"; then
    return 0
  fi
  echo "Git authority mismatch details:" >&2
  cat -- "$diagnostic" >&2
  return 1
}

capture_command() {
  local name="$1"
  shift
  set +e
  (cd "$REPOSITORY" && "$@") >"$EVIDENCE_DIR/inspection/$name.out" 2>"$EVIDENCE_DIR/inspection/$name.err"
  local status=$?
  set -e
  printf '%s\n' "$status" >"$EVIDENCE_DIR/inspection/$name.exit-status"
  return 0
}

directory_bytes() {
  local path="$1"
  if [[ ! -e "$path" ]]; then
    printf '0\n'
    return
  fi
  if du -sk -- "$path" >/dev/null 2>&1; then
    du -sk -- "$path" | awk '{print $1 * 1024}'
  else
    du -sk "$path" | awk '{print $1 * 1024}'
  fi
}

copy_if_present() {
  local source="$1"
  local destination="$2"
  if [[ ! -e "$source" && ! -L "$source" ]]; then
    return
  fi
  if [[ -L "$source" ]]; then
    return 1
  fi
  mkdir -p -- "$(dirname -- "$destination")"
  cp -a -- "$source" "$destination"
}

write_hash_inventory() {
  local root="$1"
  local inventory="$root/files.sha256"
  local bundle="$root/bundle.sha256"
  : >"$inventory"
  local file rel digest
  while IFS= read -r file; do
    rel="${file#"$root/"}"
    [[ "$rel" =~ ^[A-Za-z0-9._/-]+$ ]] || fail "evidence path is not canonical: $rel"
    regular_file_is_unaliased "$file" || fail "evidence file is aliased or unsafe: $rel"
    digest="$(hash_file "$file")"
    regular_file_is_unaliased "$file" || fail "evidence file changed identity while hashing: $rel"
    printf '%s  %s\n' "$digest" "$rel" >>"$inventory"
  done < <(find "$root" -type f ! -name files.sha256 ! -name bundle.sha256 -print | sort)
  if find "$root" -type l -print -quit | grep -q .; then
    fail "evidence bundle contains an unhashable symlink"
  fi
  regular_file_is_unaliased "$inventory" || fail "evidence inventory is aliased or unsafe"
  printf '%s  files.sha256\n' "$(hash_file "$inventory")" >"$bundle"
  regular_file_is_unaliased "$inventory" || fail "evidence inventory changed identity while hashing"
  regular_file_is_unaliased "$bundle" || fail "evidence bundle digest is aliased or unsafe"
}

verify_manifest() {
  local selected="$1"
  local root manifest inventory bundle
  if [[ -d "$selected" ]]; then
    root="$(canonical_existing_dir "$selected")" || return 1
    manifest="$root/manifest.tsv"
  else
    manifest="$selected"
    [[ -f "$manifest" && ! -L "$manifest" ]] || return 1
    root="$(canonical_existing_dir "$(dirname -- "$manifest")")" || return 1
    manifest="$root/$(basename -- "$manifest")"
  fi
  inventory="$root/files.sha256"
  bundle="$root/bundle.sha256"
  regular_file_is_unaliased "$manifest" || return 1
  regular_file_is_unaliased "$inventory" || return 1
  regular_file_is_unaliased "$bundle" || return 1
  grep -Fxq $'schema_version\t'"$SCHEMA_VERSION" "$manifest" || return 1
  local expected_inventory actual_inventory
  expected_inventory="$(awk '$2 == "files.sha256" {print $1}' "$bundle")"
  actual_inventory="$(hash_file "$inventory")"
  [[ "$expected_inventory" == "$actual_inventory" ]] || return 1
  local expected rel actual count=0
  while read -r expected rel; do
    [[ "$expected" =~ ^[0-9a-f]{64}$ && "$rel" =~ ^[A-Za-z0-9._/-]+$ ]] || return 1
    regular_file_is_unaliased "$root/$rel" || return 1
    actual="$(hash_file "$root/$rel")"
    regular_file_is_unaliased "$root/$rel" || return 1
    [[ "$expected" == "$actual" ]] || return 1
    count=$((count + 1))
  done <"$inventory"
  [[ "$count" -gt 0 ]] || return 1
  local actual_count
  actual_count="$(find "$root" -type f ! -name files.sha256 ! -name bundle.sha256 -print | wc -l | tr -d ' ')"
  [[ "$actual_count" == "$count" ]] || return 1
  if find "$root" -type l -print -quit | grep -q .; then
    return 1
  fi
  echo "Verified Level-1 dogfood evidence: $manifest"
}

verify_fixture_alias_rejection() {
  local source="$1"
  local alias_root="$TMP_ROOT/aliased-evidence"
  cp -a -- "$source" "$alias_root"
  cmp "$alias_root/identity/config-check.err" "$alias_root/identity/doctor.err" >/dev/null || return 1
  rm -- "$alias_root/identity/doctor.err"
  ln "$alias_root/identity/config-check.err" "$alias_root/identity/doctor.err"
  if verify_manifest "$alias_root" >/dev/null 2>&1; then
    return 1
  fi
}

REPOSITORY=""
CONFIRM_DISPOSABLE=""
OUTSIDE_SENTINEL=""
CANDIDATE_BINARY=""
CANDIDATE_BINARY_SHA256=""
CANDIDATE_SOURCE_COMMIT=""
CANDIDATE_VERSION_OUTPUT=""
CODEX_EXECUTABLE=""
CODEX_SHA256=""
CODEX_VERSION=""
APPROVED_CONFIG_SHA256=""
TASK_ID=""
OPERATION_ID=""
SCENARIO=""
EXPECTED_OUTCOME=""
EVIDENCE_TIME=""
EVIDENCE_DIR=""
MAX_CYCLES=50
FIXTURE_ONLY=false
FIXTURE_FAULT=""
VERIFY_MANIFEST=""

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --repository) REPOSITORY="${2:-}"; shift 2 ;;
    --confirm-disposable-repository) CONFIRM_DISPOSABLE="${2:-}"; shift 2 ;;
    --outside-sentinel) OUTSIDE_SENTINEL="${2:-}"; shift 2 ;;
    --candidate-binary) CANDIDATE_BINARY="${2:-}"; shift 2 ;;
    --candidate-binary-sha256) CANDIDATE_BINARY_SHA256="${2:-}"; shift 2 ;;
    --candidate-source-commit) CANDIDATE_SOURCE_COMMIT="${2:-}"; shift 2 ;;
    --candidate-version-output) CANDIDATE_VERSION_OUTPUT="${2:-}"; shift 2 ;;
    --codex-executable) CODEX_EXECUTABLE="${2:-}"; shift 2 ;;
    --codex-sha256) CODEX_SHA256="${2:-}"; shift 2 ;;
    --codex-version) CODEX_VERSION="${2:-}"; shift 2 ;;
    --approved-config-sha256) APPROVED_CONFIG_SHA256="${2:-}"; shift 2 ;;
    --task) TASK_ID="${2:-}"; shift 2 ;;
    --operation-id) OPERATION_ID="${2:-}"; shift 2 ;;
    --scenario) SCENARIO="${2:-}"; shift 2 ;;
    --expected-outcome) EXPECTED_OUTCOME="${2:-}"; shift 2 ;;
    --evidence-time) EVIDENCE_TIME="${2:-}"; shift 2 ;;
    --evidence-dir) EVIDENCE_DIR="${2:-}"; shift 2 ;;
    --max-cycles) MAX_CYCLES="${2:-}"; shift 2 ;;
    --fixture-only) FIXTURE_ONLY=true; shift ;;
    --fixture-fault) FIXTURE_FAULT="${2:-}"; shift 2 ;;
    --verify-manifest) VERIFY_MANIFEST="${2:-}"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

if [[ -n "$VERIFY_MANIFEST" ]]; then
  [[ "$FIXTURE_ONLY" == false && -z "$REPOSITORY$CANDIDATE_BINARY$TASK_ID$OPERATION_ID" ]] || fail "--verify-manifest is exclusive"
  verify_manifest "$VERIFY_MANIFEST" || fail "manifest verification failed: $VERIFY_MANIFEST"
  exit 0
fi

setup_fixture() {
  command -v go >/dev/null 2>&1 || fail "fixture-only requires go"
  command -v git >/dev/null 2>&1 || fail "fixture-only requires git"
  FIXTURE_ROOT="${TMPDIR:-/tmp}/revolvr-external-level1-fixture"
  FIXTURE_LOCK="$FIXTURE_ROOT.lock"
  mkdir -- "$FIXTURE_LOCK" 2>/dev/null || fail "fixture-only is already running: $FIXTURE_LOCK"
  [[ "$FIXTURE_ROOT" == "${TMPDIR:-/tmp}/revolvr-external-level1-fixture" ]] || fail "unsafe fixture root"
  rm -rf -- "$FIXTURE_ROOT"
  mkdir -p -- "$FIXTURE_ROOT/candidate-source" "$FIXTURE_ROOT/bin" "$FIXTURE_ROOT/external-project" "$FIXTURE_ROOT/outside-sentinel"

  cat >"$FIXTURE_ROOT/candidate-source/go.mod" <<'EOF'
module example.invalid/revolvrfixture

go 1.22
EOF
  cat >"$FIXTURE_ROOT/candidate-source/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const fixedTime = "2026-07-16T12:00:00Z"

func write(path, value string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { panic(err) }
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil { panic(err) }
}

func arg(name, fallback string) string {
	for i := 1; i+1 < len(os.Args); i++ { if os.Args[i] == name { return os.Args[i+1] } }
	return fallback
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" { fmt.Println("revolvr fixture-level1-v1"); return }
	cwd, _ := os.Getwd()
	if len(os.Args) >= 3 && os.Args[1] == "config" && os.Args[2] == "check" {
		fmt.Printf("Config path: %s/.revolvr/config.yaml\nConfig found: true\n", cwd)
		fmt.Printf("Codex executable: %s\n", os.Getenv("FIXTURE_CODEX_PATH"))
		fmt.Printf("Codex executable identity: version=%q configured=%q resolved=%q sha256=%s\n", os.Getenv("FIXTURE_CODEX_VERSION"), os.Getenv("FIXTURE_CODEX_PATH"), os.Getenv("FIXTURE_CODEX_PATH"), os.Getenv("FIXTURE_CODEX_SHA256"))
		fmt.Println("Effective config schema: revolvr-effective-run-config-v8")
		fmt.Println("Effective config SHA-256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		fmt.Println("Source-writer lock: timeout=32m0s heartbeat_interval=10m40s")
		fmt.Println("Autonomy mode: operator_attended")
		fmt.Println("Attended operational bounds: task_attempts=16 action_attempts=[audit=4,correct=4,document=4,implement=4,plan=4,simplify=4] elapsed=4h0m0s model_tokens=1000000 cycles_per_task=50 process_duration=30m0s output_bytes_per_stream=262144 retained_disk_bytes=1073741824 notification_attempts=0")
		fmt.Println("Verification command count: 1")
		fmt.Println("Commit allow pre-existing dirty: false")
		fmt.Println("Commit allow missing verification: false")
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "doctor" {
		fmt.Println("Dogfood preflight:")
		fmt.Println("OK mode: attended-task")
		fmt.Println("OK codex version: fixture release-authorized exact identity")
		fmt.Println("OK operational bounds: task_attempts=16 model_tokens=1000000 retained_disk_bytes=1073741824")
		fmt.Println("Ready: true")
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "run" {
		task := arg("--task", "fixture-task")
		op := arg("--operation-id", "fixture-operation")
		write(filepath.Join(cwd, "docs", "fixture-result.txt"), "fixture result\n")
		taskPath := filepath.Join(cwd, ".agent", "tasks", task+".md")
		raw, _ := os.ReadFile(taskPath)
		write(taskPath, strings.Replace(string(raw), "status: pending", "status: completed", 1))
		cmd := exec.Command("git", "add", "--", "docs/fixture-result.txt", ".agent/tasks/"+task+".md")
		cmd.Dir = cwd; if out, err := cmd.CombinedOutput(); err != nil { panic(string(out)) }
		cmd = exec.Command("git", "commit", "-q", "-m", "fixture task completion")
		cmd.Dir = cwd; cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+fixedTime, "GIT_COMMITTER_DATE="+fixedTime)
		if out, err := cmd.CombinedOutput(); err != nil { panic(string(out)) }
		head, _ := exec.Command("git", "-C", cwd, "rev-parse", "HEAD").Output()
		headText := strings.TrimSpace(string(head))
		write(filepath.Join(cwd, ".revolvr", "autonomous", "task-runs", op, "operation.json"), fmt.Sprintf("{\n  \"schema_version\": \"autonomous-task-run-operation-v1\",\n  \"operation_id\": %q,\n  \"task_id\": %q,\n  \"stage\": \"terminal\",\n  \"stop_reason\": \"completed\",\n  \"completed_at\": %q,\n  \"effective_bounds\": {\"task_attempts\":16,\"model_tokens\":1000000,\"cycles_per_task\":50,\"retained_disk_bytes\":1073741824}\n}\n", op, task, fixedTime))
		write(filepath.Join(cwd, ".revolvr", "autonomous", "task-runs", op, "history", "00000000000000000001.json"), "{\"stage\":\"terminal\",\"stop_reason\":\"completed\"}\n")
		write(filepath.Join(cwd, ".revolvr", "autonomous", "tasks", task, "state.json"), fmt.Sprintf("{\"schema_version\":\"autonomous-execution-state-v1\",\"task_id\":%q,\"lifecycle\":\"completed\"}\n", task))
		write(filepath.Join(cwd, ".revolvr", "autonomous", "tasks", task, "history", "finalization", "0001.json"), "{\"stage\":\"ledger_completed\"}\n")
		write(filepath.Join(cwd, ".revolvr", "autonomous", "tasks", task, "completion", "completion.md"), "# Fixture completion\n")
		write(filepath.Join(cwd, ".revolvr", "autonomous", "tasks", task, "completion", "completion-manifest.json"), "{\"schema_version\":\"autonomous-completion-capsule-manifest-v1\"}\n")
		write(filepath.Join(cwd, ".revolvr", "runs", "fixture-run", "provenance.json"), fmt.Sprintf("{\"run_id\":\"fixture-run\",\"commit_sha\":%q}\n", headText))
		write(filepath.Join(cwd, ".revolvr", "runs", "fixture-run", "codex.jsonl"), "{\"type\":\"fixture\"}\n")
		write(filepath.Join(cwd, ".revolvr", "receipts", "fixture-run.md"), "schema_version: revolvr.receipt.v1\nrun_id: fixture-run\nverdict: completed\nverification_status: passed\n")
		fmt.Printf("Task run: task=%s operation=%s cycles=1/50 stop=completed replayed=false\n", task, op)
		fmt.Println("Last: action=complete decision=fixture-decision run=fixture-run")
		fmt.Println("Stats: supervisors=1/1 attempts=1/1 verification=1 audits=1 corrections=0 optional=0 commits=1 checkpoints=1 actions=implement:1")
		fmt.Println("Detail: fixture completed")
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "status" { fmt.Println("Initialized: true\nRecent runs: 1\nLatest run: fixture-run (completed)"); return }
	if len(os.Args) >= 3 && os.Args[1] == "task" && os.Args[2] == "show" {
		task := os.Args[3]
		for _, value := range os.Args { if value == "--json" { fmt.Printf("{\n  \"task_id\": %q,\n  \"source_kind\": \"active\",\n  \"workspace\": {\"execution_root\": %q},\n  \"lifecycle\": \"completed\"\n}\n", task, cwd); return } }
		fmt.Printf("Task ID: %s\nLifecycle: completed\nWorkspace execution root: %s\n", task, cwd); return
	}
	if len(os.Args) >= 3 && os.Args[1] == "task" && os.Args[2] == "why" { fmt.Println("Next supervisor action: terminal"); return }
	if len(os.Args) >= 2 && os.Args[1] == "show" { fmt.Printf("Run ID: %s\nStatus: completed\n", os.Args[2]); return }
	if len(os.Args) >= 3 && os.Args[1] == "receipt" && os.Args[2] == "validate" { fmt.Println("Receipt validation: passed\nidentity: ok\nartifacts: ok"); return }
	if len(os.Args) >= 3 && os.Args[1] == "metrics" && os.Args[2] == "show" { fmt.Println("Metrics source: live\nTask operations: 1"); return }
	if len(os.Args) >= 3 && os.Args[1] == "ledger" && os.Args[2] == "export" {
		if len(os.Args) >= 4 && os.Args[3] == "verify" { fmt.Println("Ledger export verification: true"); return }
		if len(os.Args) >= 4 && os.Args[3] == "replay-validate" { fmt.Println("Ledger export replay validation: passed=true"); return }
		write(filepath.Join(cwd, ".revolvr", "retention", "exports", "fixture-export", "manifest.json"), "{\"schema_version\":\"revolvr-ledger-export-v1\",\"export_id\":\"fixture-export\"}\n")
		write(filepath.Join(cwd, ".revolvr", "retention", "exports", "fixture-export", "records.jsonl"), "{\"schema_version\":\"revolvr-ledger-export-record-v1\"}\n")
		fmt.Println("Ledger export: fixture-export"); return
	}
	fmt.Fprintln(os.Stderr, "fixture candidate: unsupported command", strings.Join(os.Args[1:], " "))
	os.Exit(64)
}
EOF
  git -C "$FIXTURE_ROOT/candidate-source" init -q
  git -C "$FIXTURE_ROOT/candidate-source" config user.name "Revolvr Fixture"
  git -C "$FIXTURE_ROOT/candidate-source" config user.email "fixture@example.invalid"
  git -C "$FIXTURE_ROOT/candidate-source" add go.mod main.go
  GIT_AUTHOR_DATE=2026-07-16T12:00:00Z GIT_COMMITTER_DATE=2026-07-16T12:00:00Z git -C "$FIXTURE_ROOT/candidate-source" commit -q -m "fixture candidate"
  (cd "$FIXTURE_ROOT/candidate-source" && go build -o "$FIXTURE_ROOT/bin/revolvr" .)

  cat >"$FIXTURE_ROOT/bin/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$#" -eq 1 && "$1" == "--version" ]]; then
  printf 'codex-cli 9.9.9-fixture\n'
  exit 0
fi
echo "fixture Codex must not execute" >&2
exit 64
EOF
  chmod 0755 "$FIXTURE_ROOT/bin/codex"

  REPOSITORY="$FIXTURE_ROOT/external-project"
  OUTSIDE_SENTINEL="$FIXTURE_ROOT/outside-sentinel"
  CANDIDATE_BINARY="$FIXTURE_ROOT/bin/revolvr"
  CODEX_EXECUTABLE="$FIXTURE_ROOT/bin/codex"
  CANDIDATE_SOURCE_COMMIT="$(git -C "$FIXTURE_ROOT/candidate-source" rev-parse HEAD)"
  CANDIDATE_BINARY_SHA256="$(hash_file "$CANDIDATE_BINARY")"
  CANDIDATE_VERSION_OUTPUT="revolvr fixture-level1-v1"
  CODEX_SHA256="$(hash_file "$CODEX_EXECUTABLE")"
  CODEX_VERSION="codex-cli 9.9.9-fixture"
  TASK_ID="fixture-task"
  OPERATION_ID="fixture-operation"
  SCENARIO="successful-source-change"
  EXPECTED_OUTCOME="completed"
  EVIDENCE_TIME="2026-07-16T12:00:00Z"
  MAX_CYCLES=50

  git -C "$REPOSITORY" init -q
  git -C "$REPOSITORY" config user.name "Revolvr External Fixture"
  git -C "$REPOSITORY" config user.email "external-fixture@example.invalid"
  printf '# External fixture\n' >"$REPOSITORY/README.md"
  printf '%s\n' "$DISPOSABLE_MARKER_CONTENT" >"$REPOSITORY/$DISPOSABLE_MARKER"
  mkdir -p -- "$REPOSITORY/.agent/tasks" "$REPOSITORY/.revolvr/autonomous/tasks/$TASK_ID"
  cat >"$REPOSITORY/.agent/tasks/$TASK_ID.md" <<EOF
---
id: $TASK_ID
status: pending
workflow: autonomous-v1
autonomous_state_path: .revolvr/autonomous/tasks/$TASK_ID/state.json
---
# Fixture task

Create docs/fixture-result.txt.
EOF
  printf '{"schema_version":"autonomous-execution-state-v1","task_id":"%s","lifecycle":"pending"}\n' "$TASK_ID" >"$REPOSITORY/.revolvr/autonomous/tasks/$TASK_ID/state.json"
  printf 'fixture-ledger-v1\n' >"$REPOSITORY/.revolvr/ledger.sqlite"
  printf '/.revolvr/\n' >>"$REPOSITORY/.git/info/exclude"
  cat >"$REPOSITORY/.revolvr/config.yaml" <<EOF
codex:
  executable: $CODEX_EXECUTABLE
  model: fixture-model
  reasoning_effort: xhigh
  ephemeral: true
autonomy:
  mode: operator_attended
verification:
  missing_policy: fail
  commands:
    - name: sh
      args: ["-c", "exit 0"]
commit:
  allow_pre_existing_dirty: false
  allow_missing_verification: false
EOF
  git -C "$REPOSITORY" add README.md "$DISPOSABLE_MARKER" .agent
  GIT_AUTHOR_DATE=2026-07-16T12:00:00Z GIT_COMMITTER_DATE=2026-07-16T12:00:00Z git -C "$REPOSITORY" commit -q -m "external fixture"
  APPROVED_CONFIG_SHA256="$(hash_file "$REPOSITORY/.revolvr/config.yaml")"
  CONFIRM_DISPOSABLE="$(canonical_existing_dir "$REPOSITORY")"

  printf 'outside sentinel\n' >"$OUTSIDE_SENTINEL/value.txt"
  printf '#!/bin/sh\nexit 0\n' >"$OUTSIDE_SENTINEL/executable.sh"
  chmod 0755 "$OUTSIDE_SENTINEL/executable.sh"
  ln "$OUTSIDE_SENTINEL/value.txt" "$OUTSIDE_SENTINEL/value-hardlink.txt"
  ln -s value.txt "$OUTSIDE_SENTINEL/value-link.txt"
}

if [[ "$FIXTURE_ONLY" == true ]]; then
  [[ -z "$REPOSITORY$CONFIRM_DISPOSABLE$OUTSIDE_SENTINEL$CANDIDATE_BINARY$CANDIDATE_BINARY_SHA256$CANDIDATE_SOURCE_COMMIT$CANDIDATE_VERSION_OUTPUT$CODEX_EXECUTABLE$CODEX_SHA256$CODEX_VERSION$APPROVED_CONFIG_SHA256$TASK_ID$OPERATION_ID$SCENARIO$EXPECTED_OUTCOME$EVIDENCE_TIME" ]] || fail "fixture-only does not accept real-operation authority options"
  setup_fixture
  if [[ -z "$EVIDENCE_DIR" ]]; then
    EVIDENCE_DIR="$(pwd -P)/dogfood-external-level1-fixture-evidence"
  fi
fi
if [[ "$FIXTURE_ONLY" == false && -n "$FIXTURE_FAULT" ]]; then
  fail "--fixture-fault requires --fixture-only"
fi

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/revolvr-external-level1-collector.XXXXXX")"

preflight() {
  local name value
  for name in REPOSITORY CONFIRM_DISPOSABLE OUTSIDE_SENTINEL CANDIDATE_BINARY CANDIDATE_BINARY_SHA256 CANDIDATE_SOURCE_COMMIT CANDIDATE_VERSION_OUTPUT CODEX_EXECUTABLE CODEX_SHA256 CODEX_VERSION APPROVED_CONFIG_SHA256 TASK_ID OPERATION_ID SCENARIO EXPECTED_OUTCOME EVIDENCE_TIME EVIDENCE_DIR; do
    value="${!name}"
    valid_safe_value "$value" || { echo "$name is required and cannot contain control whitespace" >&2; return 1; }
  done
  [[ "$MAX_CYCLES" =~ ^[1-9][0-9]*$ ]] || { echo "--max-cycles must be positive" >&2; return 1; }
  valid_sha256 "$CANDIDATE_BINARY_SHA256" || { echo "candidate SHA-256 is malformed" >&2; return 1; }
  valid_sha256 "$CODEX_SHA256" || { echo "Codex SHA-256 is malformed" >&2; return 1; }
  valid_sha256 "$APPROVED_CONFIG_SHA256" || { echo "approved config SHA-256 is malformed" >&2; return 1; }
  valid_git_oid "$CANDIDATE_SOURCE_COMMIT" || { echo "candidate source commit is malformed" >&2; return 1; }
  [[ "$TASK_ID" =~ ^[a-z0-9][a-z0-9._-]*$ && "$OPERATION_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ && "$SCENARIO" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || { echo "task, operation, or scenario identity is malformed" >&2; return 1; }
  case "$EXPECTED_OUTCOME" in completed|blocked|needs_input|budget_exhausted|no_progress|safety_stop|task_cancelled|operation_cancelled|max_cycles|unsafe_or_ambiguous) ;; *) echo "expected outcome is not a Level-1 task-operation stop" >&2; return 1 ;; esac
  [[ "$EVIDENCE_TIME" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?Z$ ]] || { echo "evidence time must be explicit RFC3339 UTC" >&2; return 1; }

  REPOSITORY="$(canonical_existing_dir "$REPOSITORY")" || { echo "repository must be an existing directory" >&2; return 1; }
  OUTSIDE_SENTINEL="$(canonical_existing_dir "$OUTSIDE_SENTINEL")" || { echo "outside sentinel must be an existing directory" >&2; return 1; }
  CONFIRM_DISPOSABLE="$(canonical_existing_dir "$CONFIRM_DISPOSABLE")" || { echo "disposable confirmation must be an existing directory" >&2; return 1; }
  EVIDENCE_DIR="$(canonical_new_path "$EVIDENCE_DIR")" || { echo "evidence parent must exist" >&2; return 1; }
  [[ "$REPOSITORY" == "$CONFIRM_DISPOSABLE" ]] || { echo "disposable confirmation does not exactly match repository" >&2; return 1; }
  [[ ! -e "$EVIDENCE_DIR" && ! -L "$EVIDENCE_DIR" ]] || { echo "evidence directory must not exist" >&2; return 1; }
  path_is_within "$OUTSIDE_SENTINEL" "$REPOSITORY" && { echo "outside sentinel is inside repository" >&2; return 1; }
  path_is_within "$REPOSITORY" "$OUTSIDE_SENTINEL" && { echo "repository is inside outside sentinel" >&2; return 1; }
  path_is_within "$EVIDENCE_DIR" "$REPOSITORY" && { echo "evidence directory is inside repository" >&2; return 1; }
  path_is_within "$EVIDENCE_DIR" "$OUTSIDE_SENTINEL" && { echo "evidence directory is inside sentinel" >&2; return 1; }
  find "$OUTSIDE_SENTINEL" -mindepth 1 -print -quit | grep -q . || { echo "outside sentinel must contain at least one entry" >&2; return 1; }

  local top bare marker marker_content
  top="$(git -C "$REPOSITORY" rev-parse --show-toplevel 2>/dev/null)" || { echo "repository is not a Git worktree" >&2; return 1; }
  top="$(canonical_existing_dir "$top")" || return 1
  [[ "$top" == "$REPOSITORY" ]] || { echo "repository must be the exact Git top-level" >&2; return 1; }
  bare="$(git -C "$REPOSITORY" rev-parse --is-bare-repository 2>/dev/null)" || return 1
  [[ "$bare" == false ]] || { echo "bare repositories are not disposable dogfood fixtures" >&2; return 1; }
  marker="$REPOSITORY/$DISPOSABLE_MARKER"
  [[ -f "$marker" && ! -L "$marker" ]] || { echo "tracked disposable marker is missing or unsafe" >&2; return 1; }
  git -C "$REPOSITORY" ls-files --error-unmatch -- "$DISPOSABLE_MARKER" >/dev/null 2>&1 || { echo "disposable marker must be tracked" >&2; return 1; }
  marker_content="$(cat -- "$marker")"
  [[ "$marker_content" == "$DISPOSABLE_MARKER_CONTENT" ]] || { echo "disposable marker content is invalid" >&2; return 1; }
  git -C "$REPOSITORY" status --porcelain=v1 -z --untracked-files=all >"$TMP_ROOT/preflight-status.bin" || return 1
  [[ ! -s "$TMP_ROOT/preflight-status.bin" ]] || { echo "repository must be clean before collection" >&2; return 1; }

  CANDIDATE_BINARY="$(canonical_existing_file "$CANDIDATE_BINARY")" || { echo "candidate must be an executable nonsymlink regular file" >&2; return 1; }
  CODEX_EXECUTABLE="$(canonical_existing_file "$CODEX_EXECUTABLE")" || { echo "Codex must be an executable nonsymlink regular file" >&2; return 1; }
  [[ -x "$CANDIDATE_BINARY" ]] || { echo "candidate must be executable" >&2; return 1; }
  [[ -x "$CODEX_EXECUTABLE" ]] || { echo "Codex must be executable" >&2; return 1; }
  [[ "$(hash_file "$CANDIDATE_BINARY")" == "$CANDIDATE_BINARY_SHA256" ]] || { echo "candidate binary SHA-256 mismatch" >&2; return 1; }
  [[ "$(hash_file "$CODEX_EXECUTABLE")" == "$CODEX_SHA256" ]] || { echo "Codex executable SHA-256 mismatch" >&2; return 1; }
  local candidate_version codex_version
  candidate_version="$(cd "$REPOSITORY" && "$CANDIDATE_BINARY" --version 2>"$TMP_ROOT/candidate-version.err")" || { echo "candidate --version failed" >&2; return 1; }
  [[ "$candidate_version" == "$CANDIDATE_VERSION_OUTPUT" ]] || { echo "candidate version output mismatch" >&2; return 1; }
  codex_version="$(cd "$REPOSITORY" && "$CODEX_EXECUTABLE" --version 2>"$TMP_ROOT/codex-version.err")" || { echo "Codex --version failed" >&2; return 1; }
  [[ "$codex_version" == "$CODEX_VERSION" ]] || { echo "Codex version mismatch" >&2; return 1; }
  command -v go >/dev/null 2>&1 || { echo "go is required to inspect candidate build metadata" >&2; return 1; }
  go version -m "$CANDIDATE_BINARY" >"$TMP_ROOT/candidate-build.txt" 2>"$TMP_ROOT/candidate-build.err" || { echo "candidate Go build metadata is unavailable" >&2; return 1; }
  local build_revision build_modified
  build_revision="$(awk '$1 == "build" && $2 ~ /^vcs.revision=/ {sub(/^vcs.revision=/, "", $2); print $2}' "$TMP_ROOT/candidate-build.txt")"
  build_modified="$(awk '$1 == "build" && $2 ~ /^vcs.modified=/ {sub(/^vcs.modified=/, "", $2); print $2}' "$TMP_ROOT/candidate-build.txt")"
  [[ "$build_revision" == "$CANDIDATE_SOURCE_COMMIT" ]] || { echo "candidate source commit mismatch or missing build metadata" >&2; return 1; }
  [[ "$build_modified" == false ]] || { echo "candidate binary was built from modified source" >&2; return 1; }

  local config="$REPOSITORY/.revolvr/config.yaml"
  [[ -f "$config" && ! -L "$config" ]] || { echo "approved config is missing or unsafe" >&2; return 1; }
  [[ "$(hash_file "$config")" == "$APPROVED_CONFIG_SHA256" ]] || { echo "approved config SHA-256 mismatch" >&2; return 1; }
  export FIXTURE_CODEX_PATH="$CODEX_EXECUTABLE" FIXTURE_CODEX_VERSION="$CODEX_VERSION" FIXTURE_CODEX_SHA256="$CODEX_SHA256"
  (cd "$REPOSITORY" && "$CANDIDATE_BINARY" config check) >"$TMP_ROOT/config-check.out" 2>"$TMP_ROOT/config-check.err" || { echo "candidate config check failed" >&2; return 1; }
  grep -Fq 'Autonomy mode: operator_attended' "$TMP_ROOT/config-check.out" || { echo "approved config is not operator_attended" >&2; return 1; }
  grep -Fq "version=\"$CODEX_VERSION\"" "$TMP_ROOT/config-check.out" || { echo "config check Codex version does not match" >&2; return 1; }
  grep -Fq "resolved=\"$CODEX_EXECUTABLE\"" "$TMP_ROOT/config-check.out" || { echo "config check Codex path does not match" >&2; return 1; }
  grep -Fq "sha256=$CODEX_SHA256" "$TMP_ROOT/config-check.out" || { echo "config check Codex digest does not match" >&2; return 1; }
  grep -Fq 'Effective config SHA-256: ' "$TMP_ROOT/config-check.out" || { echo "effective config identity is missing" >&2; return 1; }
  local expected_bounds='Attended operational bounds: task_attempts=16 action_attempts=[audit=4,correct=4,document=4,implement=4,plan=4,simplify=4] elapsed=4h0m0s model_tokens=1000000 cycles_per_task=50 process_duration=30m0s output_bytes_per_stream=262144 retained_disk_bytes=1073741824 notification_attempts=0'
  grep -Fxq "$expected_bounds" "$TMP_ROOT/config-check.out" || { echo "approved attended bounds do not exactly match the Level-1 release contract" >&2; return 1; }
  (cd "$REPOSITORY" && "$CANDIDATE_BINARY" doctor --for attended-task --task "$TASK_ID") >"$TMP_ROOT/doctor.out" 2>"$TMP_ROOT/doctor.err" || { echo "candidate doctor refused the exact task" >&2; return 1; }
  grep -Fxq 'Ready: true' "$TMP_ROOT/doctor.out" || { echo "candidate doctor did not report Ready: true" >&2; return 1; }
}

if [[ "$FIXTURE_ONLY" == true && -n "$FIXTURE_FAULT" ]]; then
  case "$FIXTURE_FAULT" in
    dirty) printf 'dirty fixture input\n' >"$REPOSITORY/dirty-input.txt" ;;
    non-disposable) rm -f -- "$REPOSITORY/$DISPOSABLE_MARKER" ;;
    wrong-binary) CANDIDATE_BINARY_SHA256="$(printf '0%.0s' {1..64})" ;;
    wrong-codex) CODEX_VERSION="codex-cli 0.0.0-wrong" ;;
    *) fail "unknown fixture fault: $FIXTURE_FAULT" ;;
  esac
  snapshot_tree "$REPOSITORY" "$TMP_ROOT/fault-repository-before.tsv" true || fail "cannot snapshot fault repository"
  snapshot_git "$REPOSITORY" "$TMP_ROOT/fault-git-before"
  snapshot_tree "$OUTSIDE_SENTINEL" "$TMP_ROOT/fault-sentinel-before.tsv" || fail "cannot snapshot fault sentinel"
  set +e
  preflight >"$TMP_ROOT/fault-preflight.out" 2>"$TMP_ROOT/fault-preflight.err"
  fault_status=$?
  set -e
  [[ "$fault_status" -ne 0 ]] || fail "fixture fault unexpectedly passed preflight: $FIXTURE_FAULT"
  snapshot_tree "$REPOSITORY" "$TMP_ROOT/fault-repository-after.tsv" true || fail "cannot resnapshot fault repository"
  snapshot_git "$REPOSITORY" "$TMP_ROOT/fault-git-after"
  snapshot_tree "$OUTSIDE_SENTINEL" "$TMP_ROOT/fault-sentinel-after.tsv" || fail "cannot resnapshot fault sentinel"
  cmp "$TMP_ROOT/fault-repository-before.tsv" "$TMP_ROOT/fault-repository-after.tsv" >/dev/null || fail "rejected $FIXTURE_FAULT input changed repository evidence"
  compare_git_snapshots "$TMP_ROOT/fault-git-before" "$TMP_ROOT/fault-git-after" || fail "rejected $FIXTURE_FAULT input changed Git authority"
  cmp "$TMP_ROOT/fault-sentinel-before.tsv" "$TMP_ROOT/fault-sentinel-after.tsv" >/dev/null || fail "rejected $FIXTURE_FAULT input changed outside sentinel evidence"
  [[ ! -e "$EVIDENCE_DIR" && ! -L "$EVIDENCE_DIR" ]] || fail "rejected $FIXTURE_FAULT input created evidence"
  echo "$SCRIPT_NAME: refused $FIXTURE_FAULT input before repository, runtime, sentinel, or evidence mutation" >&2
  exit 64
fi

preflight || fail "preflight refused the operation"

mkdir -p -- "$EVIDENCE_DIR/identity" "$EVIDENCE_DIR/before" "$EVIDENCE_DIR/operation" "$EVIDENCE_DIR/inspection" "$EVIDENCE_DIR/after" "$EVIDENCE_DIR/captured"
cp -- "$TMP_ROOT/candidate-build.txt" "$EVIDENCE_DIR/identity/candidate-build.txt"
cp -- "$TMP_ROOT/config-check.out" "$EVIDENCE_DIR/identity/config-check.out"
cp -- "$TMP_ROOT/config-check.err" "$EVIDENCE_DIR/identity/config-check.err"
cp -- "$TMP_ROOT/doctor.out" "$EVIDENCE_DIR/identity/doctor.out"
cp -- "$TMP_ROOT/doctor.err" "$EVIDENCE_DIR/identity/doctor.err"
printf '%s\n' "$CANDIDATE_VERSION_OUTPUT" >"$EVIDENCE_DIR/identity/candidate-version.txt"
printf '%s\n' "$CANDIDATE_BINARY_SHA256" >"$EVIDENCE_DIR/identity/candidate-binary.sha256"
printf '%s\n' "$CANDIDATE_SOURCE_COMMIT" >"$EVIDENCE_DIR/identity/candidate-source-commit.txt"
printf '%s\n' "$CODEX_VERSION" >"$EVIDENCE_DIR/identity/codex-version.txt"
printf '%s\n' "$CODEX_SHA256" >"$EVIDENCE_DIR/identity/codex-executable.sha256"
printf '%s\n' "$APPROVED_CONFIG_SHA256" >"$EVIDENCE_DIR/identity/approved-config.sha256"
cp -- "$REPOSITORY/.revolvr/config.yaml" "$EVIDENCE_DIR/identity/approved-config.yaml"

snapshot_git "$REPOSITORY" "$EVIDENCE_DIR/before/git"
snapshot_tree "$OUTSIDE_SENTINEL" "$EVIDENCE_DIR/before/outside-sentinel.tsv" || fail "cannot snapshot outside sentinel"
snapshot_tree "$REPOSITORY/.agent" "$EVIDENCE_DIR/before/agent-tree.tsv" || fail "cannot snapshot canonical task authority"
snapshot_tree "$REPOSITORY/.revolvr" "$EVIDENCE_DIR/before/runtime-tree.tsv" || fail "cannot snapshot runtime authority"
RUNTIME_BYTES_BEFORE="$(directory_bytes "$REPOSITORY/.revolvr")"
START_EPOCH="$(date -u +%s)"
printf '%s\n' "$START_EPOCH" >"$EVIDENCE_DIR/operation/started-epoch.txt"

RUN_INTERRUPTED=0
trap 'RUN_INTERRUPTED=1' INT
set +e
(cd "$REPOSITORY" && "$CANDIDATE_BINARY" run --until-terminal --task "$TASK_ID" --operation-id "$OPERATION_ID" --max-cycles "$MAX_CYCLES") >"$EVIDENCE_DIR/operation/run.stdout" 2>"$EVIDENCE_DIR/operation/run.stderr"
RUN_STATUS=$?
set -e
trap - INT
FINISH_EPOCH="$(date -u +%s)"
printf '%s\n' "$RUN_STATUS" >"$EVIDENCE_DIR/operation/run.exit-status"

OBSERVED_OUTCOME="$(sed -n 's/^Task run: .* stop=\([^ ]*\) replayed=.*/\1/p' "$EVIDENCE_DIR/operation/run.stdout" | head -n 1)"
if [[ -z "$OBSERVED_OUTCOME" && -f "$REPOSITORY/.revolvr/autonomous/task-runs/$OPERATION_ID/operation.json" ]]; then
  OBSERVED_OUTCOME="$(sed -n 's/.*"stop_reason"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$REPOSITORY/.revolvr/autonomous/task-runs/$OPERATION_ID/operation.json" | tail -n 1)"
fi
[[ -n "$OBSERVED_OUTCOME" ]] || OBSERVED_OUTCOME="unclassified"
printf 'schema_version\trevolvr-external-level1-typed-outcome-v1\nscenario\t%s\nexpected\t%s\nobserved\t%s\nrun_exit_status\t%s\ninterrupt_observed\t%s\n' "$SCENARIO" "$EXPECTED_OUTCOME" "$OBSERVED_OUTCOME" "$RUN_STATUS" "$RUN_INTERRUPTED" >"$EVIDENCE_DIR/operation/typed-outcome.tsv"

capture_command status "$CANDIDATE_BINARY" status
capture_command task-show "$CANDIDATE_BINARY" task show "$TASK_ID"
capture_command task-show-json "$CANDIDATE_BINARY" task show "$TASK_ID" --json
capture_command task-why "$CANDIDATE_BINARY" task why "$TASK_ID"
capture_command metrics-live "$CANDIDATE_BINARY" metrics show

RUN_IDS_FILE="$EVIDENCE_DIR/inspection/run-ids.txt"
find "$REPOSITORY/.revolvr/receipts" -maxdepth 1 -type f -name '*.md' -print 2>/dev/null | sed 's#.*/##; s/\.md$//' | sort >"$RUN_IDS_FILE" || true
while IFS= read -r run_id; do
  [[ -n "$run_id" && "$run_id" =~ ^[A-Za-z0-9._-]+$ ]] || continue
  capture_command "show-$run_id" "$CANDIDATE_BINARY" show "$run_id"
  capture_command "receipt-validate-$run_id" "$CANDIDATE_BINARY" receipt validate "$run_id"
done <"$RUN_IDS_FILE"

EXPORT_OPERATION_ID="dogfood-export-$OPERATION_ID"
set +e
(cd "$REPOSITORY" && "$CANDIDATE_BINARY" ledger export --operation-id "$EXPORT_OPERATION_ID" --exported-at "$EVIDENCE_TIME") >"$EVIDENCE_DIR/inspection/ledger-export.out" 2>"$EVIDENCE_DIR/inspection/ledger-export.err"
EXPORT_STATUS=$?
set -e
printf '%s\n' "$EXPORT_STATUS" >"$EVIDENCE_DIR/inspection/ledger-export.exit-status"
EXPORT_ID="$(awk '/^Ledger export: / {print $3; exit}' "$EVIDENCE_DIR/inspection/ledger-export.out")"
if [[ -n "$EXPORT_ID" && "$EXPORT_ID" =~ ^[A-Za-z0-9._-]+$ ]]; then
  capture_command ledger-export-verify "$CANDIDATE_BINARY" ledger export verify "$EXPORT_ID"
  capture_command ledger-export-replay "$CANDIDATE_BINARY" ledger export replay-validate "$EXPORT_ID"
  capture_command metrics-export "$CANDIDATE_BINARY" metrics show --export "$EXPORT_ID"
else
  EXPORT_ID="unclassified"
fi

snapshot_git "$REPOSITORY" "$EVIDENCE_DIR/after/git"
snapshot_tree "$OUTSIDE_SENTINEL" "$EVIDENCE_DIR/after/outside-sentinel.tsv" || fail "cannot resnapshot outside sentinel"
snapshot_tree "$REPOSITORY/.agent" "$EVIDENCE_DIR/after/agent-tree.tsv" || fail "cannot resnapshot canonical task authority"
snapshot_tree "$REPOSITORY/.revolvr" "$EVIDENCE_DIR/after/runtime-tree.tsv" || fail "cannot resnapshot runtime authority"
RUNTIME_BYTES_AFTER="$(directory_bytes "$REPOSITORY/.revolvr")"

WORKSPACE_ROOT="$(sed -n 's/.*"execution_root"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$EVIDENCE_DIR/inspection/task-show-json.out" | head -n 1)"
if [[ -n "$WORKSPACE_ROOT" && -d "$WORKSPACE_ROOT" ]]; then
  WORKSPACE_ROOT="$(canonical_existing_dir "$WORKSPACE_ROOT")"
  snapshot_git "$WORKSPACE_ROOT" "$EVIDENCE_DIR/after/workspace-git"
  snapshot_tree "$WORKSPACE_ROOT" "$EVIDENCE_DIR/after/workspace-tree.tsv" || fail "cannot snapshot task workspace"
else
  printf 'not_available\n' >"$EVIDENCE_DIR/after/workspace-tree.tsv"
fi

copy_if_present "$REPOSITORY/.agent/tasks/$TASK_ID.md" "$EVIDENCE_DIR/captured/canonical-task.md" || fail "cannot copy canonical task"
copy_if_present "$REPOSITORY/.revolvr/autonomous/tasks/$TASK_ID" "$EVIDENCE_DIR/captured/autonomous-task" || fail "cannot copy task state/history/completion"
copy_if_present "$REPOSITORY/.revolvr/autonomous/task-runs/$OPERATION_ID" "$EVIDENCE_DIR/captured/task-operation" || fail "cannot copy task operation"
copy_if_present "$REPOSITORY/.revolvr/runs" "$EVIDENCE_DIR/captured/runs" || fail "cannot copy run artifacts"
copy_if_present "$REPOSITORY/.revolvr/receipts" "$EVIDENCE_DIR/captured/receipts" || fail "cannot copy receipts"
copy_if_present "$REPOSITORY/.revolvr/retention/exports" "$EVIDENCE_DIR/captured/ledger-exports" || fail "cannot copy ledger exports"
copy_if_present "$REPOSITORY/.revolvr/ledger.sqlite" "$EVIDENCE_DIR/captured/ledger.sqlite" || fail "cannot copy live ledger evidence"
[[ -f "$EVIDENCE_DIR/captured/canonical-task.md" ]] || fail "operation left no canonical task evidence"
[[ -f "$EVIDENCE_DIR/captured/autonomous-task/state.json" ]] || fail "operation left no canonical autonomous state evidence"
[[ -f "$EVIDENCE_DIR/captured/task-operation/operation.json" ]] || fail "operation left no durable task-operation evidence"

EVIDENCE_BYTES_BEFORE_MANIFEST="$(directory_bytes "$EVIDENCE_DIR")"
printf 'schema_version\trevolvr-external-level1-resource-use-v1\nstarted_epoch\t%s\nfinished_epoch\t%s\nwall_seconds\t%s\nruntime_bytes_before\t%s\nruntime_bytes_after\t%s\nevidence_bytes_before_manifest\t%s\nrun_stdout_bytes\t%s\nrun_stderr_bytes\t%s\n' "$START_EPOCH" "$FINISH_EPOCH" "$((FINISH_EPOCH - START_EPOCH))" "$RUNTIME_BYTES_BEFORE" "$RUNTIME_BYTES_AFTER" "$EVIDENCE_BYTES_BEFORE_MANIFEST" "$(wc -c <"$EVIDENCE_DIR/operation/run.stdout" | tr -d ' ')" "$(wc -c <"$EVIDENCE_DIR/operation/run.stderr" | tr -d ' ')" >"$EVIDENCE_DIR/operation/resource-use.tsv"

SENTINEL_UNCHANGED=false
if cmp "$EVIDENCE_DIR/before/outside-sentinel.tsv" "$EVIDENCE_DIR/after/outside-sentinel.tsv" >/dev/null; then
  SENTINEL_UNCHANGED=true
fi
CANDIDATE_UNCHANGED=false
if [[ "$(hash_file "$CANDIDATE_BINARY")" == "$CANDIDATE_BINARY_SHA256" ]]; then CANDIDATE_UNCHANGED=true; fi
CODEX_UNCHANGED=false
if [[ "$(hash_file "$CODEX_EXECUTABLE")" == "$CODEX_SHA256" ]]; then CODEX_UNCHANGED=true; fi
CONFIG_UNCHANGED=false
if [[ "$(hash_file "$REPOSITORY/.revolvr/config.yaml")" == "$APPROVED_CONFIG_SHA256" ]]; then CONFIG_UNCHANGED=true; fi

{
  printf 'schema_version\t%s\n' "$SCHEMA_VERSION"
  printf 'fixture_only\t%s\n' "$FIXTURE_ONLY"
  printf 'collected_at_utc\t%s\n' "$EVIDENCE_TIME"
  printf 'scenario\t%s\n' "$SCENARIO"
  printf 'task_id\t%s\n' "$TASK_ID"
  printf 'operation_id\t%s\n' "$OPERATION_ID"
  printf 'max_cycles\t%s\n' "$MAX_CYCLES"
  printf 'expected_outcome\t%s\n' "$EXPECTED_OUTCOME"
  printf 'observed_outcome\t%s\n' "$OBSERVED_OUTCOME"
  printf 'run_exit_status\t%s\n' "$RUN_STATUS"
  printf 'repository\t%s\n' "$REPOSITORY"
  printf 'source_head_before\t%s\n' "$(cat "$EVIDENCE_DIR/before/git/head.txt")"
  printf 'source_head_after\t%s\n' "$(cat "$EVIDENCE_DIR/after/git/head.txt")"
  printf 'outside_sentinel\t%s\n' "$OUTSIDE_SENTINEL"
  printf 'outside_sentinel_unchanged\t%s\n' "$SENTINEL_UNCHANGED"
  printf 'candidate_version_output\t%s\n' "$CANDIDATE_VERSION_OUTPUT"
  printf 'candidate_binary_sha256\t%s\n' "$CANDIDATE_BINARY_SHA256"
  printf 'candidate_source_commit\t%s\n' "$CANDIDATE_SOURCE_COMMIT"
  printf 'candidate_unchanged\t%s\n' "$CANDIDATE_UNCHANGED"
  printf 'codex_version\t%s\n' "$CODEX_VERSION"
  printf 'codex_sha256\t%s\n' "$CODEX_SHA256"
  printf 'codex_unchanged\t%s\n' "$CODEX_UNCHANGED"
  printf 'approved_config_sha256\t%s\n' "$APPROVED_CONFIG_SHA256"
  printf 'approved_config_unchanged\t%s\n' "$CONFIG_UNCHANGED"
  printf 'ledger_export_id\t%s\n' "$EXPORT_ID"
  printf 'ledger_export_status\t%s\n' "$EXPORT_STATUS"
  printf 'workspace_root\t%s\n' "${WORKSPACE_ROOT:-not_available}"
} >"$EVIDENCE_DIR/manifest.tsv"

write_hash_inventory "$EVIDENCE_DIR"
verify_manifest "$EVIDENCE_DIR" >/dev/null || fail "fresh evidence bundle did not verify"
if [[ "$FIXTURE_ONLY" == true ]]; then
  verify_fixture_alias_rejection "$EVIDENCE_DIR" || fail "fixture hard-linked evidence was not rejected"
fi

FINAL_FAILURE=""
[[ "$OBSERVED_OUTCOME" == "$EXPECTED_OUTCOME" ]] || FINAL_FAILURE="observed outcome $OBSERVED_OUTCOME did not match expected $EXPECTED_OUTCOME"
[[ "$OBSERVED_OUTCOME" != unclassified ]] || FINAL_FAILURE="operation produced no typed outcome"
[[ "$SENTINEL_UNCHANGED" == true ]] || FINAL_FAILURE="outside sentinel changed"
[[ "$CANDIDATE_UNCHANGED" == true && "$CODEX_UNCHANGED" == true && "$CONFIG_UNCHANGED" == true ]] || FINAL_FAILURE="candidate, Codex, or configuration identity drifted"
[[ "$EXPORT_STATUS" == 0 && "$EXPORT_ID" != unclassified ]] || FINAL_FAILURE="ledger export failed"
for status_file in \
  "$EVIDENCE_DIR"/inspection/status.exit-status \
  "$EVIDENCE_DIR"/inspection/task-show.exit-status \
  "$EVIDENCE_DIR"/inspection/task-show-json.exit-status \
  "$EVIDENCE_DIR"/inspection/task-why.exit-status \
  "$EVIDENCE_DIR"/inspection/metrics-live.exit-status \
  "$EVIDENCE_DIR"/inspection/ledger-export-verify.exit-status \
  "$EVIDENCE_DIR"/inspection/ledger-export-replay.exit-status \
  "$EVIDENCE_DIR"/inspection/metrics-export.exit-status; do
  [[ -f "$status_file" && "$(cat "$status_file")" == 0 ]] || FINAL_FAILURE="ledger verification or replay validation failed"
done
for status_file in "$EVIDENCE_DIR"/inspection/receipt-validate-*.exit-status; do
  [[ -e "$status_file" ]] || continue
  [[ "$(cat "$status_file")" == 0 ]] || FINAL_FAILURE="one or more receipts failed validation"
done
grep -Fq 'Ledger export verification: true' "$EVIDENCE_DIR/inspection/ledger-export-verify.out" || FINAL_FAILURE="ledger export verification did not report true"
grep -Fq 'passed=true' "$EVIDENCE_DIR/inspection/ledger-export-replay.out" || FINAL_FAILURE="ledger replay validation did not report passed=true"
if [[ -n "$FINAL_FAILURE" ]]; then
  fail "$FINAL_FAILURE; evidence retained at $EVIDENCE_DIR"
fi

echo "Level-1 dogfood evidence collected: $EVIDENCE_DIR/manifest.tsv"
echo "Typed outcome: $OBSERVED_OUTCOME"
echo "Verify with: $0 --verify-manifest $EVIDENCE_DIR/manifest.tsv"
