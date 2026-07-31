#!/usr/bin/env bash
set -euo pipefail
umask 0022
export LC_ALL=C
export TZ=UTC

readonly SCRIPT_NAME="build-level1-candidate"
readonly AUTHORITY_SCHEMA="revolvr-level1-candidate-authority-v1"
readonly STATUS_SCHEMA="revolvr-level1-candidate-build-status-v1"
readonly REPRODUCIBILITY_SCHEMA="revolvr-level1-candidate-reproducibility-v1"
readonly TARGETS=(linux darwin freebsd)
readonly GO_ARCH=amd64

fail() {
	printf '%s: %s\n' "$SCRIPT_NAME" "$*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Usage:
  scripts/build-level1-candidate.sh --build \
    --candidate-id <id> --release-version <version> \
    --source-repository <clean-repository> \
    --source-commit <full-oid> --source-tree <full-oid> \
    --output-root <new-directory> \
    --floor-go <executable> --floor-go-version <go-version> \
    --current-go <executable> --current-go-version <go-version> \
    --govulncheck <executable>

  scripts/build-level1-candidate.sh --verify \
    --candidate-authority <candidate-authority.tsv> \
    --candidate-authority-sha256 <sha256>

--build runs the Go 1.22 source-floor tests, current-toolchain ordinary and
race tests, module verification, vet, ordinary and verbose vulnerability
scans, and two isolated builds for Linux, macOS, and FreeBSD amd64. The output
root must not exist. A failed build retains its output root and status.

--verify is read-only and checks the externally supplied authority hash, the
complete bundle manifest, reproducibility evidence, executable identity,
embedded source metadata, and version output. It never runs Codex.
EOF
}

hash_file() {
	sha256sum -- "$1" | awk '{print $1}'
}

canonical_dir() {
	(cd "$1" 2>/dev/null && pwd -P)
}

canonical_file() {
	local path="$1" parent
	[[ -f "$path" && ! -L "$path" ]] || return 1
	parent="$(canonical_dir "$(dirname -- "$path")")" || return 1
	printf '%s/%s\n' "$parent" "$(basename -- "$path")"
}

canonical_executable() {
	local path
	path="$(canonical_file "$1")" || return 1
	[[ -x "$path" ]] || return 1
	printf '%s\n' "$path"
}

valid_sha256() {
	[[ "$1" =~ ^[0-9a-f]{64}$ ]]
}

valid_git_oid() {
	[[ "$1" =~ ^[0-9a-f]{40}$ || "$1" =~ ^[0-9a-f]{64}$ ]]
}

valid_id() {
	[[ "$1" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]
}

valid_release_version() {
	[[ "$1" =~ ^[0-9][0-9A-Za-z.-]*$ ]]
}

safe_path_spelling() {
	[[ "$1" =~ ^/[A-Za-z0-9._/-]+$ ]]
}

valid_relative_path() {
	[[ "$1" =~ ^[A-Za-z0-9._/-]+$ && "$1" != /* && "/$1/" != *"/../"* && "/$1/" != *"/./"* ]]
}

authority_value() {
	local authority="$1" key="$2"
	awk -F '\t' -v key="$key" '$1 == key {print $2}' "$authority"
}

validate_authority_shape() {
	local authority="$1"
	awk -F '\t' '
		NF != 2 || $1 !~ /^[a-z][a-z0-9_]*$/ || seen[$1]++ { exit 1 }
		END { if (NR != 13) exit 1 }
	' "$authority" || fail "candidate authority has malformed, duplicate, or unexpected fields"
	local expected actual
	expected=$'schema_version\ncandidate_id\nrelease_version\ncandidate_version_output\ncandidate_source_commit\ncandidate_source_tree\nworkflow_sha256\ncandidate_binary\ncandidate_binary_sha256\nbundle_manifest_sha256\nfloor_go_version\ncurrent_go_version\ngovulncheck_sha256'
	actual="$(cut -f1 "$authority")"
	[[ "$actual" == "$expected" ]] || fail "candidate authority fields or ordering changed"
}

build_metadata_has() {
	local metadata="$1" key="$2" value="$3"
	awk -v key="$key" -v value="$value" '
		$1 == "build" && $2 == key "=" value { found = 1 }
		END { exit(found ? 0 : 1) }
	' "$metadata"
}

build_metadata_output_has() {
	local metadata="$1" key="$2" value="$3"
	awk -v key="$key" -v value="$value" '
		$1 == "build" && $2 == key "=" value { found = 1 }
		END { exit(found ? 0 : 1) }
	' <<<"$metadata"
}

git_repository_clean() {
	local executable="$1" repository="$2" status
	status="$("$executable" -C "$repository" status --porcelain=v1 --untracked-files=all)" || return 1
	[[ -z "$status" ]]
}

verify_bundle() {
	local selected="$1" expected_authority_sha="$2"
	local authority root manifest relative_binary binary expected_binary_sha
	local source_commit release_version version_output current_go_version
	authority="$(canonical_file "$selected")" || fail "candidate authority must be a nonsymlink regular file"
	safe_path_spelling "$authority" || fail "candidate authority path must use a simple absolute spelling"
	[[ "$(basename -- "$authority")" == candidate-authority.tsv ]] || fail "candidate authority must be named candidate-authority.tsv"
	valid_sha256 "$expected_authority_sha" || fail "candidate authority SHA-256 is malformed"
	[[ "$(hash_file "$authority")" == "$expected_authority_sha" ]] || fail "candidate authority SHA-256 mismatch"
	validate_authority_shape "$authority"
	[[ "$(authority_value "$authority" schema_version)" == "$AUTHORITY_SCHEMA" ]] || fail "candidate authority schema changed"
	valid_id "$(authority_value "$authority" candidate_id)" || fail "candidate ID is malformed"
	release_version="$(authority_value "$authority" release_version)"
	valid_release_version "$release_version" || fail "release version is malformed"
	version_output="$(authority_value "$authority" candidate_version_output)"
	[[ "$version_output" == "revolvr $release_version" ]] || fail "candidate version authority is inconsistent"
	source_commit="$(authority_value "$authority" candidate_source_commit)"
	valid_git_oid "$source_commit" || fail "candidate source commit is malformed"
	valid_git_oid "$(authority_value "$authority" candidate_source_tree)" || fail "candidate source tree is malformed"
	valid_sha256 "$(authority_value "$authority" workflow_sha256)" || fail "workflow identity is malformed"
	valid_sha256 "$(authority_value "$authority" govulncheck_sha256)" || fail "govulncheck identity is malformed"
	root="$(canonical_dir "$(dirname -- "$authority")")" || fail "candidate root is unavailable"
	manifest="$root/SHA256SUMS"
	[[ -f "$manifest" && ! -L "$manifest" ]] || fail "candidate bundle manifest is missing"
	[[ "$(hash_file "$manifest")" == "$(authority_value "$authority" bundle_manifest_sha256)" ]] || fail "candidate bundle manifest identity changed"
	[[ -z "$(find "$root" -type l -print -quit)" ]] || fail "candidate bundle contains a symbolic link"
	[[ -z "$(find "$root" -type f -links +1 -print -quit)" ]] || fail "candidate bundle contains a hard-linked file"
	cmp -s \
		<(cd "$root" && find . -type f ! -name SHA256SUMS ! -name candidate-authority.tsv -print | sort) \
		<(sed -n 's/^[0-9a-f]\{64\}  //p' "$manifest" | sort) || fail "candidate bundle manifest is not a complete file inventory"
	(cd "$root" && sha256sum --check --strict SHA256SUMS >/dev/null) || fail "candidate bundle manifest verification failed"
	relative_binary="$(authority_value "$authority" candidate_binary)"
	valid_relative_path "$relative_binary" || fail "candidate binary path is unsafe"
	binary="$(canonical_file "$root/$relative_binary")" || fail "candidate binary is missing or aliased"
	[[ "$binary" == "$root/"* && -x "$binary" ]] || fail "candidate binary escapes the bundle or is not executable"
	expected_binary_sha="$(authority_value "$authority" candidate_binary_sha256)"
	valid_sha256 "$expected_binary_sha" || fail "candidate binary identity is malformed"
	[[ "$(hash_file "$binary")" == "$expected_binary_sha" ]] || fail "candidate binary SHA-256 mismatch"
	[[ -f "$root/reproducibility.tsv" ]] || fail "candidate reproducibility evidence is missing"
	command -v go >/dev/null 2>&1 || fail "go is required to inspect candidate metadata"
	local metadata_output
	current_go_version="$(authority_value "$authority" current_go_version)"
	for target in "${TARGETS[@]}"; do
		local first="pass-1/artifacts/revolvr-v${release_version}-${target}-${GO_ARCH}"
		local second="pass-2/artifacts/revolvr-v${release_version}-${target}-${GO_ARCH}"
		cmp -s "$root/$first" "$root/$second" || fail "$target/$GO_ARCH candidate builds differ"
		awk -F '\t' -v target="$target/$GO_ARCH" -v sha="$(hash_file "$root/$first")" '
			$1 == target && $2 == sha && $3 == sha && $4 == "identical" { found = 1 }
			END { exit(found ? 0 : 1) }
		' "$root/reproducibility.tsv" || fail "$target/$GO_ARCH reproducibility record changed"
		metadata_output="$(go version -m "$root/$first")" || fail "$target/$GO_ARCH Go build metadata is unavailable"
		awk -v want="$current_go_version" 'NR == 1 { exit($NF == want ? 0 : 1) }' <<<"$metadata_output" || fail "$target/$GO_ARCH Go version metadata changed"
		build_metadata_output_has "$metadata_output" -trimpath true || fail "$target/$GO_ARCH trimpath metadata changed"
		build_metadata_output_has "$metadata_output" vcs.revision "$source_commit" || fail "$target/$GO_ARCH source revision metadata changed"
		build_metadata_output_has "$metadata_output" vcs.modified false || fail "$target/$GO_ARCH records modified source"
		build_metadata_output_has "$metadata_output" GOOS "$target" || fail "$target/$GO_ARCH target metadata changed"
		build_metadata_output_has "$metadata_output" GOARCH "$GO_ARCH" || fail "$target/$GO_ARCH architecture metadata changed"
		build_metadata_output_has "$metadata_output" CGO_ENABLED 0 || fail "$target/$GO_ARCH CGO metadata changed"
		[[ -z "$(go tool buildid "$root/$first")" ]] || fail "$target/$GO_ARCH build ID changed"
	done
	[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail "candidate execution verification requires Linux amd64"
	[[ "$("$binary" --version)" == "$version_output" ]] || fail "candidate version output changed"
	printf 'Verified Level-1 candidate authority: %s\n' "$authority"
}

record_command() {
	local evidence_root="$1" name="$2"
	shift 2
	if ! "$@" >"$evidence_root/$name.stdout" 2>"$evidence_root/$name.stderr"; then
		fail "$name failed; retained evidence under $evidence_root"
	fi
}

write_build_status() {
	local output_root="$1" candidate_id="$2" source_commit="$3" result="$4"
	{
		printf 'schema_version\t%s\n' "$STATUS_SCHEMA"
		printf 'candidate_id\t%s\n' "$candidate_id"
		printf 'source_commit\t%s\n' "$source_commit"
		printf 'result\t%s\n' "$result"
	} >"$output_root/status.tsv"
}

build_candidate() {
	local candidate_id="$1" release_version="$2" source_repository="$3"
	local source_commit="$4" source_tree="$5" output_argument="$6"
	local floor_go="$7" floor_go_version="$8" current_go="$9"
	local current_go_version="${10}" govulncheck="${11}"
	local output_parent output_root output_name git_executable workflow_source
	local workflow_hash govulncheck_hash source_epoch work_root evidence_root submodule_status

	[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail "candidate construction requires Linux amd64"
	valid_id "$candidate_id" || fail "candidate ID is malformed"
	valid_release_version "$release_version" || fail "release version is malformed"
	valid_git_oid "$source_commit" || fail "source commit must be a full SHA-1 or SHA-256 object ID"
	valid_git_oid "$source_tree" || fail "source tree must be a full SHA-1 or SHA-256 object ID"
	[[ "$floor_go_version" =~ ^go1\.22\.[0-9]+$ ]] || fail "floor Go version must be an exact patched Go 1.22 version"
	[[ "$current_go_version" =~ ^go[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "current Go version must be exact"
	source_repository="$(canonical_dir "$source_repository")" || fail "source repository does not exist"
	git_executable="$(canonical_executable "$(command -v git)")" || fail "git executable is unavailable"
	floor_go="$(canonical_executable "$floor_go")" || fail "floor Go executable is unavailable"
	current_go="$(canonical_executable "$current_go")" || fail "current Go executable is unavailable"
	govulncheck="$(canonical_executable "$govulncheck")" || fail "govulncheck executable is unavailable"
	[[ "$(env GOTOOLCHAIN=local "$floor_go" env GOVERSION)" == "$floor_go_version" ]] || fail "floor Go executable reports the wrong version"
	[[ "$(env GOTOOLCHAIN=local "$current_go" env GOVERSION)" == "$current_go_version" ]] || fail "current Go executable reports the wrong version"
	[[ "$("$git_executable" -C "$source_repository" rev-parse --verify HEAD)" == "$source_commit" ]] || fail "source repository HEAD does not equal the exact source commit"
	[[ "$("$git_executable" -C "$source_repository" rev-parse --verify 'HEAD^{tree}')" == "$source_tree" ]] || fail "source repository tree does not equal the exact source tree"
	[[ "$("$git_executable" -C "$source_repository" rev-parse --is-bare-repository)" == false ]] || fail "source repository must be non-bare"
	git_repository_clean "$git_executable" "$source_repository" || fail "source repository must be clean"
	submodule_status="$("$git_executable" -C "$source_repository" submodule status --recursive)" || fail "source submodule authority is unreadable"
	[[ -z "$submodule_status" ]] || fail "source repository must not contain active submodules"
	workflow_source="$(canonical_file "${BASH_SOURCE[0]}")" || fail "workflow source is unavailable"
	cmp -s "$workflow_source" <("$git_executable" -C "$source_repository" show "$source_commit:scripts/build-level1-candidate.sh") || fail "running workflow does not equal the exact source-commit workflow"
	workflow_hash="$(hash_file "$workflow_source")"
	govulncheck_hash="$(hash_file "$govulncheck")"
	output_parent="$(canonical_dir "$(dirname -- "$output_argument")")" || fail "output parent does not exist"
	output_name="$(basename -- "$output_argument")"
	valid_id "$output_name" || fail "output directory name is unsafe"
	output_root="$output_parent/$output_name"
	safe_path_spelling "$output_root" || fail "output root must use a simple absolute spelling"
	[[ ! -e "$output_root" && ! -L "$output_root" ]] || fail "output root already exists"
	mkdir "$output_root"
	work_root="$output_root/.work"
	evidence_root="$output_root/evidence"
	mkdir -p "$work_root" "$evidence_root/tests" "$evidence_root/tools" \
		"$output_root/pass-1/artifacts" "$output_root/pass-1/metadata" \
		"$output_root/pass-2/artifacts" "$output_root/pass-2/metadata"

	local status_trap
	printf -v status_trap 'write_build_status %q %q %q failed' \
		"$output_root" "$candidate_id" "$source_commit"
	trap "$status_trap" EXIT

	"$git_executable" --version >"$evidence_root/tools/git-version.txt"
	env GOTOOLCHAIN=local "$floor_go" version >"$evidence_root/tools/floor-go-version.txt"
	env GOTOOLCHAIN=local "$floor_go" env >"$evidence_root/tools/floor-go-env.txt"
	env GOTOOLCHAIN=local "$current_go" version >"$evidence_root/tools/current-go-version.txt"
	env GOTOOLCHAIN=local "$current_go" env >"$evidence_root/tools/current-go-env.txt"
	"$govulncheck" -version >"$evidence_root/tools/govulncheck-version.txt" 2>&1
	printf '%s  %s\n' "$(hash_file "$git_executable")" "$git_executable" >"$evidence_root/tools/git-executable.sha256"
	printf '%s  %s\n' "$(hash_file "$floor_go")" "$floor_go" >"$evidence_root/tools/floor-go-executable.sha256"
	printf '%s  %s\n' "$(hash_file "$current_go")" "$current_go" >"$evidence_root/tools/current-go-executable.sha256"
	printf '%s  %s\n' "$govulncheck_hash" "$govulncheck" >"$evidence_root/tools/govulncheck-executable.sha256"

	local pass source_dir
	for pass in 1 2; do
		source_dir="$work_root/source-$pass"
		"$git_executable" -c protocol.file.allow=always clone --quiet --no-local --no-checkout "$source_repository" "$source_dir"
		"$git_executable" -C "$source_dir" checkout --quiet --detach "$source_commit"
		[[ "$("$git_executable" -C "$source_dir" rev-parse --verify HEAD)" == "$source_commit" ]] || fail "source pass $pass commit changed"
		[[ "$("$git_executable" -C "$source_dir" rev-parse --verify 'HEAD^{tree}')" == "$source_tree" ]] || fail "source pass $pass tree changed"
		git_repository_clean "$git_executable" "$source_dir" || fail "source pass $pass is dirty or unreadable"
	done
	source_epoch="$("$git_executable" -C "$work_root/source-1" show -s --format=%ct "$source_commit")"
	{
		printf 'schema_version\trevolvr-level1-candidate-effective-build-v1\n'
		printf 'candidate_id\t%s\n' "$candidate_id"
		printf 'release_version\t%s\n' "$release_version"
		printf 'source_commit\t%s\n' "$source_commit"
		printf 'source_tree\t%s\n' "$source_tree"
		printf 'source_date_epoch\t%s\n' "$source_epoch"
		printf 'workflow_sha256\t%s\n' "$workflow_hash"
		printf 'floor_go_version\t%s\n' "$floor_go_version"
		printf 'current_go_version\t%s\n' "$current_go_version"
		printf 'govulncheck_sha256\t%s\n' "$govulncheck_hash"
		printf 'targets\tlinux/amd64,darwin/amd64,freebsd/amd64\n'
		printf 'cgo_enabled\t0\n'
		printf 'goamd64\tv1\n'
		printf 'goenv\toff\n'
		printf 'goexperiment\t\n'
		printf 'goflags\t\n'
		printf 'gotoolchain\tlocal\n'
		printf 'gowork\toff\n'
		printf 'locale\tC\n'
		printf 'timezone\tUTC\n'
		printf 'build_flags\t-mod=readonly -trimpath -buildvcs=true\n'
		printf 'ldflags\t-buildid= -X main.version=%s\n' "$release_version"
	} >"$evidence_root/effective-build.tsv"

	local floor_env=(env CGO_ENABLED=1 GOAMD64=v1 GOENV=off GOEXPERIMENT= GOFLAGS= GOTOOLCHAIN=local GOWORK=off \
		GOCACHE="$work_root/floor-go-cache" GOMODCACHE="$work_root/floor-go-mod-cache" \
		SOURCE_DATE_EPOCH="$source_epoch" PATH="$(dirname -- "$floor_go"):$PATH")
	local current_env=(env CGO_ENABLED=1 GOAMD64=v1 GOENV=off GOEXPERIMENT= GOFLAGS= GOTOOLCHAIN=local GOWORK=off \
		GOCACHE="$work_root/current-go-cache" GOMODCACHE="$work_root/current-go-mod-cache" \
		SOURCE_DATE_EPOCH="$source_epoch" PATH="$(dirname -- "$current_go"):$PATH")
	(
		cd "$work_root/source-1"
		record_command "$evidence_root/tests" floor-go-test \
			"${floor_env[@]}" "$floor_go" test -count=1 ./...
		record_command "$evidence_root/tests" current-go-test \
			"${current_env[@]}" "$current_go" test -count=1 ./...
		record_command "$evidence_root/tests" current-go-race \
			"${current_env[@]}" "$current_go" test -race -count=1 ./...
		record_command "$evidence_root/tests" current-go-mod-verify \
			"${current_env[@]}" "$current_go" mod verify
		record_command "$evidence_root/tests" current-go-vet \
			"${current_env[@]}" "$current_go" vet ./...
		record_command "$evidence_root/tests" govulncheck \
			"${current_env[@]}" "$govulncheck" ./...
		record_command "$evidence_root/tests" govulncheck-verbose \
			"${current_env[@]}" "$govulncheck" -show verbose ./...
	)

	printf 'schema_version\t%s\n' "$REPRODUCIBILITY_SCHEMA" >"$output_root/reproducibility.tsv"
	printf 'target\tpass_1_sha256\tpass_2_sha256\tresult\n' >>"$output_root/reproducibility.tsv"
	local target artifact_name artifact metadata build_id actual_hash first_hash second_hash
	for pass in 1 2; do
		source_dir="$work_root/source-$pass"
		for target in "${TARGETS[@]}"; do
			artifact_name="revolvr-v${release_version}-${target}-${GO_ARCH}"
			artifact="$output_root/pass-$pass/artifacts/$artifact_name"
			metadata="$output_root/pass-$pass/metadata/$artifact_name.buildinfo.txt"
			(
				cd "$source_dir"
				env CGO_ENABLED=0 GOAMD64=v1 GOARCH="$GO_ARCH" GOENV=off GOEXPERIMENT= GOFLAGS= GOOS="$target" \
					GOTOOLCHAIN=local GOWORK=off \
					GOCACHE="$work_root/build-go-cache-$pass" \
					GOMODCACHE="$work_root/build-go-mod-cache-$pass" \
					SOURCE_DATE_EPOCH="$source_epoch" PATH="$(dirname -- "$current_go"):$PATH" \
					"$current_go" build -mod=readonly -trimpath -buildvcs=true \
					-ldflags="-buildid= -X main.version=$release_version" \
					-o "$artifact" ./cmd/revolvr
			)
			"$current_go" version -m "$artifact" >"$metadata"
			awk -v want="$current_go_version" 'NR == 1 { exit($NF == want ? 0 : 1) }' "$metadata" || fail "$artifact_name has the wrong Go version"
			awk '$1 == "path" && $2 == "revolvr/cmd/revolvr" { found = 1 } END { exit(found ? 0 : 1) }' "$metadata" || fail "$artifact_name has the wrong package path"
			build_metadata_has "$metadata" -trimpath true || fail "$artifact_name lacks trimpath metadata"
			build_metadata_has "$metadata" GOOS "$target" || fail "$artifact_name has the wrong target OS"
			build_metadata_has "$metadata" GOARCH "$GO_ARCH" || fail "$artifact_name has the wrong target architecture"
			build_metadata_has "$metadata" CGO_ENABLED 0 || fail "$artifact_name enables CGO"
			build_metadata_has "$metadata" vcs.revision "$source_commit" || fail "$artifact_name has the wrong source revision"
			build_metadata_has "$metadata" vcs.modified false || fail "$artifact_name records modified source"
			build_id="$("$current_go" tool buildid "$artifact")"
			[[ -z "$build_id" ]] || fail "$artifact_name has a nonempty build ID"
			printf '%s' "$build_id" >"$output_root/pass-$pass/metadata/$artifact_name.build-id.txt"
		done
		git_repository_clean "$git_executable" "$source_dir" || fail "source pass $pass changed or became unreadable during construction"
	done

	for target in "${TARGETS[@]}"; do
		artifact_name="revolvr-v${release_version}-${target}-${GO_ARCH}"
		first_hash="$(hash_file "$output_root/pass-1/artifacts/$artifact_name")"
		second_hash="$(hash_file "$output_root/pass-2/artifacts/$artifact_name")"
		cmp -s "$output_root/pass-1/artifacts/$artifact_name" "$output_root/pass-2/artifacts/$artifact_name" || fail "$target/$GO_ARCH builds are not reproducible"
		printf '%s/%s\t%s\t%s\tidentical\n' "$target" "$GO_ARCH" "$first_hash" "$second_hash" >>"$output_root/reproducibility.tsv"
	done
	artifact_name="revolvr-v${release_version}-linux-${GO_ARCH}"
	artifact="$output_root/pass-1/artifacts/$artifact_name"
	[[ "$("$artifact" --version)" == "revolvr $release_version" ]] || fail "candidate version output is wrong"
	printf 'revolvr %s\n' "$release_version" >"$output_root/pass-1/metadata/linux-version-output.txt"
	actual_hash="$(hash_file "$artifact")"

	chmod -R u+w "$work_root"
	find "$work_root" -depth -delete
	write_build_status "$output_root" "$candidate_id" "$source_commit" passed
	(
		cd "$output_root"
		find . -type f ! -name SHA256SUMS ! -name candidate-authority.tsv -print0 | sort -z | while IFS= read -r -d '' path; do
			sha256sum -- "$path"
		done >SHA256SUMS
	)
	local manifest_hash
	manifest_hash="$(hash_file "$output_root/SHA256SUMS")"
	{
		printf 'schema_version\t%s\n' "$AUTHORITY_SCHEMA"
		printf 'candidate_id\t%s\n' "$candidate_id"
		printf 'release_version\t%s\n' "$release_version"
		printf 'candidate_version_output\trevolvr %s\n' "$release_version"
		printf 'candidate_source_commit\t%s\n' "$source_commit"
		printf 'candidate_source_tree\t%s\n' "$source_tree"
		printf 'workflow_sha256\t%s\n' "$workflow_hash"
		printf 'candidate_binary\tpass-1/artifacts/%s\n' "$artifact_name"
		printf 'candidate_binary_sha256\t%s\n' "$actual_hash"
		printf 'bundle_manifest_sha256\t%s\n' "$manifest_hash"
		printf 'floor_go_version\t%s\n' "$floor_go_version"
		printf 'current_go_version\t%s\n' "$current_go_version"
		printf 'govulncheck_sha256\t%s\n' "$govulncheck_hash"
	} >"$output_root/candidate-authority.tsv"
	local authority_hash
	authority_hash="$(hash_file "$output_root/candidate-authority.tsv")"
	verify_bundle "$output_root/candidate-authority.tsv" "$authority_hash" >/dev/null
	trap - EXIT
	printf 'Built Level-1 candidate: %s\n' "$output_root"
	printf 'Candidate authority: %s\n' "$output_root/candidate-authority.tsv"
	printf 'Candidate authority SHA-256: %s\n' "$authority_hash"
	printf 'Dogfood authority options: --candidate-authority %q --candidate-authority-sha256 %q\n' \
		"$output_root/candidate-authority.tsv" "$authority_hash"
}

MODE=""
CANDIDATE_ID=""
RELEASE_VERSION=""
SOURCE_REPOSITORY=""
SOURCE_COMMIT=""
SOURCE_TREE=""
OUTPUT_ROOT=""
FLOOR_GO=""
FLOOR_GO_VERSION=""
CURRENT_GO=""
CURRENT_GO_VERSION=""
GOVULNCHECK=""
CANDIDATE_AUTHORITY=""
CANDIDATE_AUTHORITY_SHA256=""

while [[ "$#" -gt 0 ]]; do
	case "$1" in
	--build|--verify)
		[[ -z "$MODE" ]] || fail "select exactly one mode"
		MODE="$1"
		shift
		;;
	--candidate-id) CANDIDATE_ID="${2:-}"; shift 2 ;;
	--release-version) RELEASE_VERSION="${2:-}"; shift 2 ;;
	--source-repository) SOURCE_REPOSITORY="${2:-}"; shift 2 ;;
	--source-commit) SOURCE_COMMIT="${2:-}"; shift 2 ;;
	--source-tree) SOURCE_TREE="${2:-}"; shift 2 ;;
	--output-root) OUTPUT_ROOT="${2:-}"; shift 2 ;;
	--floor-go) FLOOR_GO="${2:-}"; shift 2 ;;
	--floor-go-version) FLOOR_GO_VERSION="${2:-}"; shift 2 ;;
	--current-go) CURRENT_GO="${2:-}"; shift 2 ;;
	--current-go-version) CURRENT_GO_VERSION="${2:-}"; shift 2 ;;
	--govulncheck) GOVULNCHECK="${2:-}"; shift 2 ;;
	--candidate-authority) CANDIDATE_AUTHORITY="${2:-}"; shift 2 ;;
	--candidate-authority-sha256) CANDIDATE_AUTHORITY_SHA256="${2:-}"; shift 2 ;;
	--help|-h) usage; exit 0 ;;
	*) fail "unknown argument: $1" ;;
	esac
done

command -v awk >/dev/null 2>&1 || fail "awk is required"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required"

case "$MODE" in
--build)
	[[ -z "$CANDIDATE_AUTHORITY$CANDIDATE_AUTHORITY_SHA256" ]] || fail "--build does not accept verification authority options"
	for required in CANDIDATE_ID RELEASE_VERSION SOURCE_REPOSITORY SOURCE_COMMIT SOURCE_TREE OUTPUT_ROOT FLOOR_GO FLOOR_GO_VERSION CURRENT_GO CURRENT_GO_VERSION GOVULNCHECK; do
		[[ -n "${!required}" ]] || fail "--build requires --$(printf '%s' "$required" | tr '[:upper:]_' '[:lower:]-')"
	done
	build_candidate "$CANDIDATE_ID" "$RELEASE_VERSION" "$SOURCE_REPOSITORY" \
		"$SOURCE_COMMIT" "$SOURCE_TREE" "$OUTPUT_ROOT" "$FLOOR_GO" \
		"$FLOOR_GO_VERSION" "$CURRENT_GO" "$CURRENT_GO_VERSION" "$GOVULNCHECK"
	;;
--verify)
	[[ -n "$CANDIDATE_AUTHORITY" && -n "$CANDIDATE_AUTHORITY_SHA256" ]] || fail "--verify requires both candidate authority options"
	[[ -z "$CANDIDATE_ID$RELEASE_VERSION$SOURCE_REPOSITORY$SOURCE_COMMIT$SOURCE_TREE$OUTPUT_ROOT$FLOOR_GO$FLOOR_GO_VERSION$CURRENT_GO$CURRENT_GO_VERSION$GOVULNCHECK" ]] || fail "--verify accepts only candidate authority options"
	verify_bundle "$CANDIDATE_AUTHORITY" "$CANDIDATE_AUTHORITY_SHA256"
	;;
*) usage >&2; exit 64 ;;
esac
