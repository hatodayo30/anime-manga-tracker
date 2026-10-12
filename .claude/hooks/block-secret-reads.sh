#!/usr/bin/env bash
# PreToolUse hook: 認証情報ファイルの読み取りをブロックする。
#
# block-destructive.sh が塞いだのは「AWS を壊す」経路だけで、「認証情報を読む」経路は
# 別に空いている。Orca はエージェントを bypass permissions モードで起動し、
# ワークツリーはサンドボックスではなく手元のマシンそのものなので、
# ~/.aws/credentials も ~/.ssh も素のまま見える。
#
# フックは権限モードに関係なく走るため、bypass 下でもここは効く。
# permissions.deny（.claude/settings.local.json）と二重に掛けている。
#
# **これは隔離ではない。** 読み取りコマンドの名前を列挙して弾いているだけなので、
# 列挙から漏れた方法（自作スクリプト経由など）はすり抜ける。本当に隔離するなら
# コンテナか VM が要る。事故と「ついでに読んでしまう」のを防ぐための柵。
set -euo pipefail

payload=$(cat)

# 守る対象。$HOME を実行時に解決するので、このファイルに個人のパスは埋め込まない。
protected=(
  "/.aws"
  "/.ssh"
  "/.config/gh"
  "/.gnupg"
  "/.netrc"
  "/.docker/config.json"
)

deny() {
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":%s}}\n' \
    "$(printf '%s' "$1" | jq -Rs .)"
  exit 0
}

# text の中に守る対象のパスが現れるか
touches_secret() {
  local text="$1" p
  for p in "${protected[@]}"; do
    if printf '%s' "$text" | grep -qF "$p"; then
      printf '%s' "$p"
      return 0
    fi
  done
  return 1
}

tool=$(printf '%s' "$payload" | jq -r '.tool_name // empty' 2>/dev/null || true)

case "$tool" in
  Read | Glob | Grep | NotebookEdit)
    target=$(printf '%s' "$payload" | jq -r '.tool_input.file_path // .tool_input.path // empty' 2>/dev/null || true)
    [ -z "$target" ] && exit 0
    if hit=$(touches_secret "$target"); then
      deny "認証情報ファイルの読み取りはブロックされています（${hit}）。
このリポジトリの作業に認証情報は必要ありません。AWS の操作は docs/DEPLOY-AWS.md を見ながら人間が実行してください。
ブロックした対象: ${target}"
    fi
    ;;

  Bash)
    command=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
    [ -z "$command" ] && exit 0

    # block-destructive.sh と同じ理由で、コマンド全体ではなくセグメントの先頭を見る。
    # コミットメッセージ本文に ~/.aws と書いただけで弾かれるのを避けるため、
    # 「ファイルを読むコマンド」で始まるセグメントに限って判定する。
    segments=$(printf '%s' "$command" | tr '\n' ' ' | sed -E 's/(\&\&|\|\||[;&|(){}])/\n/g')

    readers='cat|bat|less|more|head|tail|nl|strings|xxd|od|base64|cp|mv|scp|rsync|install|tee|grep|rg|ag|awk|sed|jq|yq|python3?|ruby|perl|node|open|code|vim|nano|emacs|source|\.'

    while IFS= read -r segment; do
      segment=$(printf '%s' "$segment" | sed -E 's/^[[:space:]]+//; s/^(sudo|env|time|nohup|command)[[:space:]]+//')
      [ -z "$segment" ] && continue

      if printf '%s' "$segment" | grep -Eq "^(${readers})([[:space:]]|$)"; then
        if hit=$(touches_secret "$segment"); then
          deny "認証情報ファイルの読み取りはブロックされています（${hit}）。
このリポジトリの作業に認証情報は必要ありません。
ブロックしたコマンド: ${segment}"
        fi
      fi
    done <<EOF
$segments
EOF
    ;;
esac

exit 0
