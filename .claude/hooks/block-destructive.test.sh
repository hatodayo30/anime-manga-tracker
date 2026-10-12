#!/usr/bin/env bash
# block-destructive.sh の回帰テスト。
#
#   bash .claude/hooks/block-destructive.test.sh
#
# このフックは一度、コマンド全体を正規表現で検索していたために
# コミットメッセージ本文の "aws" を拾って自分自身のコミットを弾いた。
# 判定を緩めたり締めたりするときは必ずここを通すこと。
#
# 注意: このファイル自身に危険なコマンド文字列が入っているので、ケースを
# 追加するときはエディタで書くこと（シェルのヒアドキュメントで書こうとすると
# フックが自分を弾く）。
set -uo pipefail

cd "$(dirname "$0")/../.."
hook=".claude/hooks/block-destructive.sh"

cases=(
  # 壊す操作
  'DENY|aws ecs delete-service --cluster amt --service amt'
  'DENY|aws rds delete-db-instance --db-instance-identifier amt'
  'DENY|cd infra && aws ecs update-service --desired-count 0'
  'DENY|sudo aws iam delete-user --user-name x'
  'DENY|docker compose down -v'
  'DENY|docker compose down --volumes'
  'DENY|docker volume rm anime-manga-tracker_pgdata'
  'DENY|docker system prune -a'
  'DENY|terraform destroy'

  # 通ってほしい操作
  'ALLOW|aws ecs describe-tasks --cluster amt'
  'ALLOW|aws ecs list-tasks --cluster amt'
  'ALLOW|aws sts get-caller-identity'
  'ALLOW|aws --version'
  'ALLOW|docker compose down'
  'ALLOW|docker compose up -d postgres'
  'ALLOW|terraform plan'
  'ALLOW|go test ./...'
  'ALLOW|git status'

  # 誤検知の回帰テスト: 文字列の中に現れた aws を拾わないこと
  'ALLOW|echo "aws を変更する操作をブロックした"'
  'ALLOW|grep -rn "aws" docs/'
)

pass=0
fail=0
for entry in "${cases[@]}"; do
  want="${entry%%|*}"
  cmd="${entry#*|}"
  out=$(printf '{"tool_name":"Bash","tool_input":{"command":%s}}' \
    "$(printf '%s' "$cmd" | jq -Rs .)" | bash "$hook")
  if [ -z "$out" ]; then got=ALLOW; else got=DENY; fi

  if [ "$got" = "$want" ]; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    printf 'FAIL want=%s got=%s  %s\n' "$want" "$got" "$cmd"
  fi
done

printf '%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
