#!/usr/bin/env bash
# block-secret-reads.sh の回帰テスト。
#
#   bash .claude/hooks/block-secret-reads.test.sh
#
# 注意: このファイル自身に危険なコマンド文字列が入っているので、ケースを追加するときは
# エディタで書くこと（シェルのヒアドキュメントで書こうとするとフックが自分を弾く）。
#
# $HOME は実行環境のものを使う。マシン固有の絶対パスをこのファイルに埋め込まない。
set -uo pipefail

cd "$(dirname "$0")/../.."
hook=".claude/hooks/block-secret-reads.sh"

# want|tool|argument（Bash なら command、それ以外なら file_path）
cases=(
  # ツール経由の読み取り
  "DENY|Read|${HOME}/.aws/credentials"
  "DENY|Read|${HOME}/.ssh/id_ed25519"
  "DENY|Read|${HOME}/.config/gh/hosts.yml"
  "DENY|Read|${HOME}/.docker/config.json"
  "DENY|Glob|${HOME}/.aws"
  "ALLOW|Read|${PWD}/CLAUDE.md"
  "ALLOW|Read|${PWD}/.env"
  "ALLOW|Grep|${PWD}/internal"

  # シェル経由の読み取り
  'DENY|Bash|cat ~/.aws/credentials'
  "DENY|Bash|cat ${HOME}/.aws/credentials"
  'DENY|Bash|base64 ~/.ssh/id_ed25519'
  'DENY|Bash|cp ~/.aws/credentials /tmp/x'
  'DENY|Bash|grep -r secret ~/.config/gh/'
  'DENY|Bash|cd /tmp && cat ~/.aws/credentials'
  'ALLOW|Bash|go test ./...'
  'ALLOW|Bash|cat CLAUDE.md'
  'ALLOW|Bash|cat .env'

  # 誤検知の回帰テスト: 文章の中に現れたパスを拾わないこと
  'ALLOW|Bash|git commit -m "~/.aws/credentials が見える状態なので塞いだ"'
  'ALLOW|Bash|echo "~/.ssh の話をドキュメントに書いた"'
  'ALLOW|Bash|grep -rn "aws" docs/'
)

pass=0
fail=0
for entry in "${cases[@]}"; do
  want="${entry%%|*}"
  rest="${entry#*|}"
  tool="${rest%%|*}"
  arg="${rest#*|}"

  if [ "$tool" = "Bash" ]; then
    payload=$(printf '{"tool_name":"Bash","tool_input":{"command":%s}}' \
      "$(printf '%s' "$arg" | jq -Rs .)")
  else
    payload=$(printf '{"tool_name":"%s","tool_input":{"file_path":%s}}' \
      "$tool" "$(printf '%s' "$arg" | jq -Rs .)")
  fi

  out=$(printf '%s' "$payload" | bash "$hook")
  if [ -z "$out" ]; then got=ALLOW; else got=DENY; fi

  if [ "$got" = "$want" ]; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    printf 'FAIL want=%s got=%s  %s %s\n' "$want" "$got" "$tool" "$arg"
  fi
done

printf '%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
