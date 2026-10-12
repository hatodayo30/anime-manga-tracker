#!/usr/bin/env bash
# PreToolUse hook: AWS リソースを壊す操作と、ローカル Postgres のボリューム削除をブロックする。
#
# フックは権限モードに関係なく実行されるので、--dangerously-skip-permissions
# （Orca がエージェントを起動するときのモード）でもこの判定は効く。
#
# 読み取り専用の AWS 操作（describe-* / list-* / get-*）は通す。デプロイ作業の
# 調査でよく使ううえ、壊しようがないため。手順書どおりに人間が自分のターミナルで
# 実行する分にはフックを通らないので、この制限はエージェントにしか掛からない。
#
# **判定はコマンドの先頭トークンに対してのみ行う。** コマンド全体を正規表現で
# 検索すると、ヒアドキュメントやコミットメッセージの本文に現れた "aws" を
# 拾って誤って弾く（実際に踏んだ）。
#
# これは万能の防御ではなく、事故を減らすための柵。ヒアドキュメント本文の行頭に
# コマンドを書くなど、すり抜ける書き方は残っている。
set -euo pipefail

payload=$(cat)
command=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null || true)

[ -z "$command" ] && exit 0

deny() {
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":%s}}\n' \
    "$(printf '%s' "$1" | jq -Rs .)"
  exit 0
}

# 改行を潰してからコマンド区切りで分割する。改行を先に潰すのは、ヒアドキュメントの
# 本文がセグメントの先頭に来ないようにするため。
segments=$(printf '%s' "$command" | tr '\n' ' ' | sed -E 's/(\&\&|\|\||[;&|(){}])/\n/g')

while IFS= read -r segment; do
  # 先頭の空白と、コマンドの前に付きうる修飾子を剥がす
  segment=$(printf '%s' "$segment" | sed -E 's/^[[:space:]]+//; s/^(sudo|env|time|nohup|command)[[:space:]]+//')
  [ -z "$segment" ] && continue

  seg_matches() {
    printf '%s' "$segment" | grep -Eq "$1"
  }

  # aws は読み取り専用のサブコマンド以外を拒否する
  if seg_matches '^aws([[:space:]]|$)'; then
    if ! seg_matches '^aws[[:space:]]+(--version|help|[a-z0-9-]+[[:space:]]+(describe|list|get)-)'; then
      deny "AWS を変更する操作はこのリポジトリでは実行できません（読み取り専用の describe-* / list-* / get-* のみ許可）。
ECS / RDS の操作は docs/DEPLOY-AWS.md を見ながら人間が実行してください。
ブロックしたコマンド: ${segment}"
    fi
  fi

  # Postgres のデータは pgdata という名前付きボリュームに入っている。これを消すと
  # ライブラリの記録が全部消えるが、エージェントからは「環境を作り直す」操作に見える。
  if seg_matches '^docker([[:space:]]+compose)?[[:space:]]+down([[:space:]]|$).*(-v([[:space:]]|$)|--volumes)'; then
    deny "docker compose down -v は pgdata ボリュームごと削除するため禁止しています。
コンテナだけ落とすなら -v を外してください。
ブロックしたコマンド: ${segment}"
  fi

  if seg_matches '^docker[[:space:]]+volume[[:space:]]+(rm|prune)'; then
    deny "docker volume rm / prune は pgdata を消す可能性があるため禁止しています。
ブロックしたコマンド: ${segment}"
  fi

  if seg_matches '^docker[[:space:]]+system[[:space:]]+prune'; then
    deny "docker system prune はボリュームを巻き込む可能性があるため禁止しています。
ブロックしたコマンド: ${segment}"
  fi

  # Terraform は BACKLOG.md 「3. AWS へのデプロイ」の IaC 化で入る予定。
  if seg_matches '^terraform[[:space:]]+(destroy|apply)'; then
    deny "terraform destroy / apply は人間が実行してください。plan までなら実行できます。
ブロックしたコマンド: ${segment}"
  fi
done <<EOF
$segments
EOF

exit 0
