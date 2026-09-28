#!/usr/bin/env bash
# PreToolUse hook: blocks Edit/Write/NotebookEdit and `git commit` while on main/master.
set -euo pipefail

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  exit 0
fi

branch=$(git branch --show-current 2>/dev/null || true)

if [ "$branch" = "main" ] || [ "$branch" = "master" ]; then
  cat <<JSON
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"You are on the '$branch' branch. Create and check out a feature branch first (e.g. git checkout -b feat/<short-description>), then retry this action."}}
JSON
fi

exit 0
