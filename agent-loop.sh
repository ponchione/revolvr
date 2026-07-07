#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

MAX_RUNS="${1:-3}"

for i in $(seq 1 "$MAX_RUNS"); do
  echo "===== Fresh Codex pass $i / $MAX_RUNS ====="
  ./agent-one.sh

  echo
  echo "Review current changes before continuing."
  git status --short 2>/dev/null || true
  echo

  read -r -p "Run another fresh pass? [y/N] " answer
  case "$answer" in
    y|Y|yes|YES) continue ;;
    *) break ;;
  esac
done
