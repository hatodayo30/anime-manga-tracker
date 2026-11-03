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

# AWS CLI v2 は長い出力を less に渡す。describe-tasks の表やログで止まったように見えるのを防ぐ
export AWS_PAGER=""

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
EXIT_CODE=$(aws ecs describe-tasks --region "$REGION" --cluster "$CLUSTER" --tasks "$TASK_ARN" \
  --query 'tasks[0].containers[0].exitCode' --output text)

echo "== ログ"
# イメージの取得や SSM の読み込みで失敗するとコンテナが起動せず、ログストリームができない。
# そのときもここで止めず、上の表の stoppedReason を見るよう促して終了コードの判定まで進む
aws logs get-log-events --region "$REGION" --log-group-name "$LOG_GROUP" \
  --log-stream-name "psql/psql/${TASK_ARN##*/}" --query 'events[].message' --output text 2>/dev/null \
  | tr '\t' '\n' \
  || echo "（ログが無い。コンテナが起動していない可能性が高いので、上の表の stoppedReason を見る）"

# psql が失敗してもタスクの停止自体は正常なので、ここで判定しないとスクリプトは成功で終わってしまう。
# 起動しなかったとき exitCode は None になる
if [[ "$EXIT_CODE" != "0" ]]; then
  echo "== 失敗（exitCode: $EXIT_CODE）。--single-transaction なので、このファイルの変更は何も残っていない"
  exit 1
fi
echo "== 成功（exitCode: 0）"
