#!/usr/bin/env bash
# PreToolUse hook: blocks Edit/Write/NotebookEdit and `git commit` while on main/master.
#
# Bash では settings.json の `"if": "Bash(git commit*)"` で git commit のときだけ呼ばれる想定だが、
# この中でもコマンドを見て git commit 以外は通す。`${VAR:-}` のような展開を含むコマンドで、
# git commit ではないのにこのフックが呼ばれ、main 上の Bash がすべて止まったため（2026-10-24）。
# `if` が展開を含むコマンドを判定できずに安全側に倒しているのだと推測しているが、確かめてはいない。
# どちらにしても、判定をこの中に持てば settings.json の `if` の解釈に左右されない。
#
# 判定の仕方は block-destructive.sh と同じく、コマンド区切りで分割した各セグメントの先頭だけを見る
# （コミットメッセージの本文に現れた "git commit" を拾わないため）。
set -euo pipefail

payload=$(cat)

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  exit 0
fi

branch=$(git branch --show-current 2>/dev/null || true)
if [ "$branch" != "main" ] && [ "$branch" != "master" ]; then
  exit 0
fi

tool=$(printf '%s' "$payload" | jq -r '.tool_name // empty' 2>/dev/null || true)
if [ "$tool" = "Bash" ]; then
  command=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
  segments=$(printf '%s' "$command" | tr '\n' ' ' | sed -E 's/(\&\&|\|\||[;&|(){}])/\n/g')
  is_commit=false
  while IFS= read -r segment; do
    segment=$(printf '%s' "$segment" | sed -E 's/^[[:space:]]+//')
    if printf '%s' "$segment" | grep -Eq '^git[[:space:]]+commit([[:space:]]|$)'; then
      is_commit=true
    fi
  done <<EOF
$segments
EOF
  [ "$is_commit" = true ] || exit 0
fi

cat <<JSON
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"You are on the '$branch' branch. Create and check out a feature branch first (e.g. git checkout -b feat/<short-description>), then retry this action."}}
JSON

exit 0
