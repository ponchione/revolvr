#!/usr/bin/env bash
set -euo pipefail

readonly RETIREMENT_NOTICE="retired terminal authority; RC.5 has no check or live execution path"

main() {
	printf 'RC.5 live gate: %s\n' "$RETIREMENT_NOTICE" >&2
	exit 1
}

main "$@"
