#!/usr/bin/env bash
# block-main-branch.sh の回帰テスト。
#
#   bash .claude/hooks/block-main-branch.test.sh
#
# 一時ディレクトリに main と作業ブランチを持つリポジトリを作り、その中でフックを呼ぶ。
# このフックは一度、git commit 以外の Bash まで main 上で止めた（2026-10-24）。
set -uo pipefail

hook="$(cd "$(dirname "$0")" && pwd)/block-main-branch.sh"
repo=$(mktemp -d)
trap 'rm -rf "$repo"' EXIT
git -C "$repo" init -q -b main
git -C "$repo" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init

cases=(
  # main 上
  'main|DENY|Edit|'
  'main|DENY|Write|'
  'main|DENY|Bash|git commit -m "x"'
  'main|DENY|Bash|git add -A && git commit -q -F -'
  'main|ALLOW|Bash|git status'
  'main|ALLOW|Bash|echo "${ORCA_CLI_COMMAND:-}"; orca skills get orca-cli'
  'main|ALLOW|Bash|git checkout -b feat/x'
  # 誤検知の回帰テスト: 文字列の中に現れた git commit を拾わないこと
  'main|ALLOW|Bash|echo "git commit は作業ブランチで"'

  # 作業ブランチ上
  'feat|ALLOW|Edit|'
  'feat|ALLOW|Bash|git commit -m "x"'
)

pass=0
fail=0
for entry in "${cases[@]}"; do
  IFS='|' read -r branch want tool cmd <<<"$entry"
  if [ "$branch" = "main" ]; then
    git -C "$repo" checkout -q main
  else
    git -C "$repo" checkout -q -B feat/test
  fi
  out=$(printf '{"tool_name":"%s","tool_input":{"command":%s}}' \
    "$tool" "$(printf '%s' "$cmd" | jq -Rs .)" | (cd "$repo" && bash "$hook"))
  if [ -z "$out" ]; then got=ALLOW; else got=DENY; fi

  if [ "$got" = "$want" ]; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    printf 'FAIL branch=%s want=%s got=%s  %s %s\n' "$branch" "$want" "$got" "$tool" "$cmd"
  fi
done

printf '%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
