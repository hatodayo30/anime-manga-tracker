#!/usr/bin/env bash
# 本番の RDS に SQL ファイルを 1 回だけ流す。amt-psql タスク（psql 入りのイメージ）を run-task で起動し、
# 終わるまで待って終了コードとログを出す。手順は docs/DEPLOY-AWS.md の「DB に SQL を流す（amt-psql）」。
#
#   bash deploy/run-sql.sh deploy/sql/check-migrations.sql
#
# - RDS はパブリックアクセスが無く、SG はアプリのタスクの SG からの 5432 しか通さない。手元からは繋がらないので、
#   アプリと同じサブネット・SG でタスクを立てる（値は実行のたびに amt-service から引く）
# - アプリのイメージは非 root で psql も無いため、ECS Exec で入っても SQL は叩けない
# - SQL は base64 にして環境変数 SQL_B64 で渡す（JSON とシェルのクォートが二重になって壊れるのを避ける）
# - psql は --single-transaction と ON_ERROR_STOP=1 で動く。どこかで失敗すればファイル全体が巻き戻る
# - -a で流した SQL もログに出す（何を流したかが CloudWatch に残る）
set -euo pipefail

SQL_FILE=${1:?usage: bash deploy/run-sql.sh <file.sql>}
REGION=${AWS_REGION:-ap-northeast-1}
CLUSTER=amt-cluster
SERVICE=amt-service
LOG_GROUP=/ecs/anime-manga-tracker

NET=$(aws ecs describe-services --region "$REGION" --cluster "$CLUSTER" --services "$SERVICE" \
  --query 'services[0].networkConfiguration.awsvpcConfiguration' --output json)
SUBNETS=$(jq -r '.subnets | join(",")' <<<"$NET")
SGS=$(jq -r '.securityGroups | join(",")' <<<"$NET")

SQL_B64=$(base64 < "$SQL_FILE" | tr -d '\n')
OVERRIDES=$(jq -n --arg b64 "$SQL_B64" \
  '{containerOverrides:[{name:"psql",environment:[{name:"SQL_B64",value:$b64}]}]}')

echo "== 流す SQL: $SQL_FILE"
cat "$SQL_FILE"
echo
read -r -p "本番の DB に流します。よろしいですか？ [y/N] " ans
[[ "$ans" == "y" ]] || { echo "中止しました"; exit 1; }

TASK_ARN=$(aws ecs run-task --region "$REGION" --cluster "$CLUSTER" \
  --task-definition amt-psql --launch-type FARGATE \
  --network-configuration "awsvpcConfiguration={subnets=[$SUBNETS],securityGroups=[$SGS],assignPublicIp=ENABLED}" \
  --overrides "$OVERRIDES" \
  --query 'tasks[0].taskArn' --output text)
echo "== 起動: $TASK_ARN"

aws ecs wait tasks-stopped --region "$REGION" --cluster "$CLUSTER" --tasks "$TASK_ARN"

aws ecs describe-tasks --region "$REGION" --cluster "$CLUSTER" --tasks "$TASK_ARN" \
  --query 'tasks[0].{stoppedReason:stoppedReason,exitCode:containers[0].exitCode,reason:containers[0].reason}' \
  --output table

echo "== ログ"
aws logs get-log-events --region "$REGION" --log-group-name "$LOG_GROUP" \
  --log-stream-name "psql/psql/${TASK_ARN##*/}" --query 'events[].message' --output text | tr '\t' '\n'
