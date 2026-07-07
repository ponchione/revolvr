#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [ ! -f ".agent/LOOP_PROMPT.md" ]; then
  echo "Missing .agent/LOOP_PROMPT.md"
  exit 1
fi

echo "Starting one fresh Codex loop pass in: $ROOT"
echo

codex exec \
  --dangerously-bypass-approvals-and-sandbox \
  --cd "$ROOT" \
  "$(cat .agent/LOOP_PROMPT.md)"

echo
echo "Codex pass complete. Changed files:"
git status --short 2>/dev/null || true
