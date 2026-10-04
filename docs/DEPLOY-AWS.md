# AWS デプロイ手順書（Phase 1: ECS Fargate / ALB なし）

インフラ作業がはじめての人が、**上から順にそのまま実行すればデプロイできる**ことを目的にした文書。
何を作るかという**判断は済んでいる**（`BACKLOG.md` の「3. AWS へのデプロイ」）ので、ここでは
やり方だけを書く。なぜその構成なのか（なぜ NAT を作らないのか、なぜ水平スケールしないのか）は
`BACKLOG.md` を読むこと。

対象は **Phase 1 = 平文 HTTP の学習用の足場**。常用する構成ではない。HTTPS 化（ALB・独自ドメイン）は
Phase 2 で、この文書の最後に道筋だけ書いてある。

最終更新: 2026-10-04

---

## 0. この作業で作るもの

```
                    インターネット
                         │
                         │  http://<タスクのパブリックIP>:8080
                         │  （SG で自分の IP だけに開ける）
                    ┌────▼─────────────────────────────────┐
                    │ VPC  10.0.0.0/16                     │
                    │                                      │
                    │  パブリックサブネット (2 AZ)          │
                    │   ┌──────────────────────────┐       │
                    │   │ ECS Fargate Spot タスク×1 │       │
                    │   │  anime-manga-tracker:8080│       │
                    │   └───────────┬──────────────┘       │
                    │               │ 5432                 │
                    │  プライベートサブネット (2 AZ)        │
                    │   ┌───────────▼──────────────┐       │
                    │   │ RDS PostgreSQL 16        │       │
                    │   │  db.t4g.micro            │       │
                    │   └──────────────────────────┘       │
                    └──────────────────────────────────────┘

        ECR（イメージ置き場） / SSM Parameter Store（DATABASE_URL）
        / CloudWatch Logs（アプリのログ）
```

**NAT ゲートウェイは作らない**。タスクはパブリックサブネットに直接置き、パブリック IP を付けて
インターネット（ECR・SSM・CloudWatch）に出る。これが月 $33 を節約する代わりに、
「タスクにパブリック IP が付いていないと何も動かない」という Phase 1 固有の制約を生む。

### 作業の順番と依存関係

後の手順が前の手順の出力（ID や ARN）を使うので、**この順番を崩さないこと**。

| Step | 作るもの | 次に何で使うか |
| --- | --- | --- |
| 0 | アカウント準備・請求アラート・CLI | 全部 |
| 1 | VPC とサブネット | SG・RDS・サービス |
| 2 | セキュリティグループ 2 つ | RDS・サービス |
| 3 | RDS | DATABASE_URL の組み立て |
| 4 | SSM パラメータ（DATABASE_URL） | タスク定義 |
| 5 | ECR とイメージ push | タスク定義 |
| 6 | IAM ロール 2 つ | タスク定義 |
| 7 | CloudWatch ロググループ | タスク定義 |
| 8 | ECS クラスタ | サービス |
| 9 | タスク定義 | サービス |
| 10 | ECS サービス | 動作確認 |

### 所要時間とお金

- 所要時間: 初回は **3〜4 時間**（RDS の作成だけで 10〜15 分待つ）
- 月額: **$20 前後**（主因は RDS。使わないときの止め方は「運用」に書いた）
- 作業中に課金が始まるのは Step 3（RDS）から。Step 1・2 は無料

### 前提

- Docker Desktop が動いていること（このマシンは **Apple Silicon / arm64** なので、Step 5 の
  `--platform linux/amd64` が必須。ここが一番ハマる）
- AWS CLI v2 が入っていること（`aws --version` で確認済み。認証はまだ未設定）
- リージョンは **`ap-northeast-1`（東京）** を使う。この文書は全部これ前提

### 作業中ずっと使う環境変数

ターミナルを開き直すたびに、これを流してから作業する。

```bash
export AWS_REGION=ap-northeast-1
export AWS_PAGER=""   # CLI の出力が less に飲まれるのを止める
export ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
echo $ACCOUNT_ID      # 12桁の数字が出れば認証OK（Step 0-3 の後）
```

---

## Step 0: アカウントと手元の準備

### 0-1. 請求アラートを先に作る（最優先）

**何かを作る前にこれをやる。** 消し忘れで数万円、は初回に最も起きやすい事故で、
アラートが無いと翌月の請求書まで気付けない。

1. マネジメントコンソール右上のアカウント名 → **請求とコスト管理**
2. 左メニュー **請求設定** → 「**請求アラートを受け取る**」にチェック → 保存
3. 左メニュー **予算** → **予算を作成**
   - テンプレート: **月次コスト予算**
   - 予算額: **30 USD**（Phase 1 の想定 $20 に少し余裕を持たせる）
   - メールアドレス: 自分のアドレス
4. 作成

> 実際のしきい値到達メールは実績が積み上がってから来る。**毎週 1 回は請求ダッシュボードを
> 自分で見る**習慣にするのが確実。

### 0-2. 作業用の IAM ユーザーを作る

ルートユーザー（サインアップに使ったメールアドレス）では日常作業をしない。ルートは
権限が無制限で、漏れたときに止める手段が無い。

1. **IAM** コンソール → **ユーザー** → **ユーザーを作成**
2. ユーザー名: `admin-haruto`
3. 「**AWS マネジメントコンソールへのユーザーアクセスを提供する**」にチェック →
   「**IAM ユーザーを作成します**」を選択 → パスワードを設定
4. 許可: **ポリシーを直接アタッチ** → `AdministratorAccess`
   - 学習用の個人アカウントなので管理者権限でよい。最小権限は Terraform 化のときに考える
5. 作成後、**MFA を設定する**（ユーザー詳細 → セキュリティ認証情報 → MFA デバイスの割り当て →
   認証アプリ）
6. 同じ画面で **アクセスキーを作成**
   - ユースケース: **コマンドラインインターフェイス (CLI)**
   - アクセスキー ID と**シークレットアクセスキー**が表示される。
     **シークレットはこの画面でしか見られない**ので、ここでコピーしておく

以降はこの IAM ユーザーでコンソールにサインインする（サインイン URL は
`https://<ACCOUNT_ID>.signin.aws.amazon.com/console`）。

### 0-3. AWS CLI に認証情報を設定

```bash
aws configure
# AWS Access Key ID     : 0-2 でコピーしたもの
# AWS Secret Access Key : 0-2 でコピーしたもの
# Default region name   : ap-northeast-1
# Default output format : json
```

確認:

```bash
aws sts get-caller-identity
# {
#   "UserId": "AIDA...",
#   "Account": "123456789012",
#   "Arn": "arn:aws:iam::123456789012:user/admin-haruto"
# }
```

> アクセスキーは `~/.aws/credentials` に平文で置かれる。このリポジトリには絶対に入れないこと
> （`.gitignore` に `.env` はあるが `~/.aws` はそもそも別の場所）。

---

## Step 1: VPC を作る

コンソールのウィザードを使うと、サブネット・ルートテーブル・インターネットゲートウェイを
まとめて作ってくれる。

1. **VPC** コンソール → **VPC を作成**
2. 「**VPC など**」を選択（「VPC のみ」ではない）
3. 設定:

| 項目 | 値 | 理由 |
| --- | --- | --- |
| 名前タグの自動生成 | `amt` | 以降すべてのリソース名が `amt-` で始まる |
| IPv4 CIDR ブロック | `10.0.0.0/16` | 既定のまま |
| IPv6 CIDR ブロック | なし | |
| テナンシー | デフォルト | |
| アベイラビリティーゾーンの数 | **2** | RDS のサブネットグループが 2 AZ を要求する |
| パブリックサブネットの数 | **2** | ECS タスクを置く |
| プライベートサブネットの数 | **2** | RDS を置く |
| **NAT ゲートウェイ** | **なし** | **月 $33。ここを間違えると費用が倍以上になる** |
| **VPC エンドポイント** | **なし** | Interface 型は 1 つ月 $7〜 |
| DNS ホスト名を有効化 | **✓** | RDS のエンドポイント名を引くのに要る |
| DNS 解決を有効化 | **✓** | 同上 |

4. **VPC を作成**

できたサブネットの ID を控えておく（Step 3・10 で使う）。

```bash
aws ec2 describe-subnets \
  --filters "Name=tag:Name,Values=amt-subnet-*" \
  --query 'Subnets[].{Name:Tags[?Key==`Name`]|[0].Value,Id:SubnetId,AZ:AvailabilityZone}' \
  --output table
```

`amt-subnet-public1-*` / `public2-*` が ECS 用、`private1-*` / `private2-*` が RDS 用。

---

## Step 2: セキュリティグループを 2 つ作る

**タスク用を先に作る。** RDS 用がタスク用を参照するため、逆順だと作れない。

### 2-1. タスク用 SG（`amt-task-sg`）

1. **VPC** コンソール → **セキュリティグループ** → **セキュリティグループを作成**
2. 名前: `amt-task-sg` / 説明: `ECS task` / VPC: **amt-vpc**
3. **インバウンドルール** → ルールを追加
   - タイプ: **カスタム TCP** / ポート: **8080** / ソース: **マイ IP**
4. アウトバウンドルール: 既定（すべて許可）のまま
   - **ここを絞らないこと。** ECR からのイメージ取得・SSM・CloudWatch・AniList/Jikan への
     アクセスが全部ここを通る
5. 作成

> **自宅の IP は変わる。** 繋がらなくなったらまずこのルールの「マイ IP」を更新する。

### 2-2. RDS 用 SG（`amt-rds-sg`）

1. 同じく **セキュリティグループを作成**
2. 名前: `amt-rds-sg` / 説明: `RDS postgres` / VPC: **amt-vpc**
3. **インバウンドルール**
   - タイプ: **PostgreSQL**（ポート 5432 が自動で入る）
   - ソース: **カスタム** → 検索窓に `amt-task-sg` と入力して**選択する**
   - **IP アドレスではなく SG を指定する。** タスクの IP は毎回変わるので IP 指定は機能しない
4. 作成

---

## Step 3: RDS（PostgreSQL）を作る

**ここから課金が始まる。** 作成完了まで 10〜15 分かかるので、待っている間に Step 5（ECR）を
進めてよい。

1. **RDS** コンソール → **データベースの作成**
2. **標準作成** を選択（簡易作成だとサブネットや SG を選べない）

| 項目 | 値 | 注意 |
| --- | --- | --- |
| エンジン | **PostgreSQL** | |
| バージョン | 16.x の最新 | ローカルの `postgres:16-alpine` に合わせる |
| テンプレート | **開発/テスト** | 本番を選ぶとマルチ AZ が既定で入って倍額 |
| 可用性と耐久性 | **単一 DB インスタンス** | |
| DB インスタンス識別子 | `amt-db` | |
| マスターユーザー名 | `anime_manga` | |
| 認証情報管理 | **セルフマネージド** | Secrets Manager は $0.40/件・月 |
| マスターパスワード | 自分で生成 | **英数字のみにすること**（後述） |
| インスタンスクラス | **バースト可能クラス → db.t4g.micro** | |
| ストレージタイプ | **gp3** | |
| ストレージ割り当て | **20 GiB** | |
| **ストレージの自動スケーリング** | **チェックを外す** | 意図しない課金増を止める |
| コンピューティングリソース | EC2 コンピューティングリソースに接続しない | |
| ネットワークタイプ | IPv4 | |
| VPC | **amt-vpc** | |
| DB サブネットグループ | 新規作成 | **プライベートサブネットが選ばれていることを確認** |
| **パブリックアクセス** | **なし** | 手元から直接は繋がらなくなる（意図どおり） |
| VPC セキュリティグループ | **既存を選択 → `amt-rds-sg`**、`default` は**外す** | |
| アベイラビリティーゾーン | 指定なし | |
| データベース認証 | パスワード認証 | |
| **モニタリング → 拡張モニタリング** | **無効** | 有料 |
| **Performance Insights** | **無効** | |

3. **「追加設定」を開く**（折りたたまれている。**ここが一番の落とし穴**）

| 項目 | 値 | 注意 |
| --- | --- | --- |
| **最初のデータベース名** | **`anime_manga_tracker`** | **空のままだと DB が作られず、アプリが起動時に落ちる** |
| 自動バックアップ | 有効 / 保持期間 **1 日** | 学習用なので最小 |
| 暗号化 | 有効（既定キー） | 無料 |
| マイナーバージョン自動アップグレード | 有効 | |
| 削除保護 | **無効** | あとで消せるようにしておく |

4. **データベースの作成**

> **パスワードに記号を使わない理由**: `DATABASE_URL` は URL なので、`@` `/` `#` `:` などが
> 入ると区切り文字として解釈されて接続に失敗する。URL エンコードすれば使えるが、
> 学習の本題ではないところで時間を溶かすので英数字 20 文字程度にしておく。

### 3-1. エンドポイントを控える

ステータスが **利用可能** になったら:

```bash
aws rds describe-db-instances --db-instance-identifier amt-db \
  --query 'DBInstances[0].Endpoint.Address' --output text
# amt-db.cxxxxxxxxxxx.ap-northeast-1.rds.amazonaws.com
```

---

## Step 4: DATABASE_URL を SSM Parameter Store に入れる

パスワードをタスク定義に平文で書かないための手順。タスク定義は IAM さえあれば誰でも読めるし、
Terraform 化したときにリポジトリに入ってしまう。

### 4-1. 接続文字列を組み立てる

```
postgres://anime_manga:<パスワード>@<RDSのエンドポイント>:5432/anime_manga_tracker?sslmode=require
```

**`sslmode=require` を必ず付ける。** PostgreSQL 15 以降の RDS は既定のパラメータグループで
`rds.force_ssl=1` が有効になっており、SSL なしの接続は拒否される。ローカルの
`docker-compose.yml` は `sslmode=disable` なので、そのままコピーすると
`pg_hba.conf entry ... SSL off` で起動に失敗する。

### 4-2. パラメータを作る

```bash
aws ssm put-parameter \
  --name /amt/prod/DATABASE_URL \
  --type SecureString \
  --value 'postgres://anime_manga:PASSWORD@amt-db.xxxx.ap-northeast-1.rds.amazonaws.com:5432/anime_manga_tracker?sslmode=require'
```

- **シングルクォートで囲む**（`?` や `&` をシェルに食われないため）
- **型は `SecureString`**。暗号化キーは既定の `alias/aws/ssm` を使う（無料）
- **標準パラメータは無料**。4KB まで

ARN を控える（Step 9 で使う）:

```bash
echo "arn:aws:ssm:${AWS_REGION}:${ACCOUNT_ID}:parameter/amt/prod/DATABASE_URL"
```

> このコマンドは**シェルの履歴にパスワードが残る**。気になるなら
> `aws ssm put-parameter --value "$(cat /tmp/url.txt)"` のようにファイル経由にして、
> あとでファイルを消す。

---

## Step 5: ECR にイメージを push する

### 5-1. リポジトリを作る

```bash
aws ecr create-repository \
  --repository-name anime-manga-tracker \
  --image-scanning-configuration scanOnPush=true
```

### 5-2. ライフサイクルポリシー（古いイメージを自動削除）

これが無いと push するたびにイメージが溜まって、ストレージ課金が増え続ける。

```bash
aws ecr put-lifecycle-policy \
  --repository-name anime-manga-tracker \
  --lifecycle-policy-text '{
    "rules": [{
      "rulePriority": 1,
      "description": "最新10イメージのみ保持",
      "selection": {
        "tagStatus": "any",
        "countType": "imageCountMoreThan",
        "countNumber": 10
      },
      "action": { "type": "expire" }
    }]
  }'
```

### 5-3. ビルドして push

```bash
export ECR_REPO=${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/anime-manga-tracker
export TAG=$(git rev-parse --short HEAD)

# ログイン（12時間で切れる。切れたら再実行）
aws ecr get-login-password --region ${AWS_REGION} \
  | docker login --username AWS --password-stdin ${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com

# ビルド（--platform を忘れないこと）
docker build --platform linux/amd64 -t ${ECR_REPO}:${TAG} .

docker push ${ECR_REPO}:${TAG}
echo "イメージURI: ${ECR_REPO}:${TAG}"   # Step 9 で使う
```

> ### `--platform linux/amd64` について
>
> このマシンは Apple Silicon（arm64）なので、何も指定しないと **arm64 のイメージができる**。
> Fargate Spot は X86_64 前提で進めるので、そのまま動かすと
> `exec format error` でタスクが起動と停止を延々と繰り返す。
> **エラーメッセージからは原因がまったく読み取れない**ので、ここで必ず付けること。
>
> ビルドはエミュレーション経由になるので遅い（初回 5〜10 分）。
> Graviton（ARM64）にして速く・安くしたくなったら、作業時点で Fargate **Spot** が
> ARM64 に対応しているかを確認してから切り替える（対応していなければ Spot を諦めて
> オンデマンドにするかの判断になる）。

> **タグに `latest` を使わない理由**: ECS は「タスク定義のイメージ URI が変わったか」で
> 入れ替えを判断する。`latest` 固定だと URI が変わらず、新しいイメージを push しても
> 古いタスクがそのまま残ったり、どのコミットが動いているのか分からなくなる。
> コミット SHA をタグにしておくと、`git log` と本番が一対一で対応する。

---

## Step 6: IAM ロールを 2 つ作る

**役割が違う 2 つのロールを混同しないこと。** ここが ECS で最初に分からなくなる場所。

| ロール | 誰が使うか | 何のため |
| --- | --- | --- |
| **タスク実行ロール** | **ECS のエージェント**（コンテナが起動する**前**） | ECR からイメージを取る、SSM から秘密を読む、ログを書く |
| **タスクロール** | **コンテナの中のアプリ自身** | アプリが AWS API を呼ぶとき。今回は ECS Exec のためだけ |

### 6-1. タスク実行ロール `amtEcsTaskExecutionRole`

1. **IAM** → **ロール** → **ロールを作成**
2. 信頼されたエンティティ: **AWS のサービス** → ユースケース: **Elastic Container Service** →
   **Elastic Container Service Task**
3. 許可ポリシー: `AmazonECSTaskExecutionRolePolicy` を検索してチェック
4. ロール名: `amtEcsTaskExecutionRole` → 作成
5. 作成したロールを開く → **許可を追加** → **インラインポリシーを作成** → **JSON**:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["ssm:GetParameters"],
    "Resource": "arn:aws:ssm:ap-northeast-1:<ACCOUNT_ID>:parameter/amt/prod/*"
  }]
}
```

   ポリシー名: `amtReadSsmParams` → 作成

> `AmazonECSTaskExecutionRolePolicy` には SSM の読み取りが**含まれていない**。
> これを足さないと `ResourceInitializationError: unable to pull secrets` でタスクが起動しない。
> 既定の KMS キー（`alias/aws/ssm`）を使っている限り `kms:Decrypt` は不要。

### 6-2. タスクロール `amtEcsTaskRole`

1. 同じ手順でロールを作成（信頼されたエンティティは同じ **Elastic Container Service Task**）
2. 許可ポリシーは**何も選ばず**先に進む
3. ロール名: `amtEcsTaskRole` → 作成
4. **インラインポリシーを作成** → **JSON**:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "ssmmessages:CreateControlChannel",
      "ssmmessages:CreateDataChannel",
      "ssmmessages:OpenControlChannel",
      "ssmmessages:OpenDataChannel"
    ],
    "Resource": "*"
  }]
}
```

   ポリシー名: `amtEcsExec` → 作成

> これが ECS Exec（実行中のコンテナにシェルで入る）の権限。**実行ロールではなくタスクロール**に
> 付ける。踏み台 EC2 を立てずに済ませるための唯一の手段なので、忘れると困る。

---

## Step 7: CloudWatch ロググループを先に作る

```bash
aws logs create-log-group --log-group-name /ecs/anime-manga-tracker
aws logs put-retention-policy --log-group-name /ecs/anime-manga-tracker --retention-in-days 14
```

> **順番が大事**。ECS に自動作成させると**保持期間が「無期限」**になり、ログが永久に課金され続ける。
> 先に作って保持期間を付けておく。

---

## Step 8: ECS クラスタを作る

```bash
aws ecs create-cluster \
  --cluster-name amt-cluster \
  --capacity-providers FARGATE FARGATE_SPOT \
  --settings name=containerInsights,value=disabled
```

- `FARGATE_SPOT` を**ここで有効化しておく**。クラスタに登録されていないとサービスで選べない
- **Container Insights は無効**（有効にすると CloudWatch のカスタムメトリクス課金が乗る）

---

## Step 9: タスク定義を登録する

コンソールのフォームでも作れるが、項目が多く、**デプロイ戦略やヘルスチェックの設定を
取りこぼしやすい**ので JSON で登録する。

`<ACCOUNT_ID>` と `<TAG>` を自分の値に置き換えてファイルを作る:

```bash
cat > /tmp/amt-taskdef.json <<EOF
{
  "family": "anime-manga-tracker",
  "requiresCompatibilities": ["FARGATE"],
  "networkMode": "awsvpc",
  "cpu": "256",
  "memory": "512",
  "runtimePlatform": {
    "cpuArchitecture": "X86_64",
    "operatingSystemFamily": "LINUX"
  },
  "executionRoleArn": "arn:aws:iam::${ACCOUNT_ID}:role/amtEcsTaskExecutionRole",
  "taskRoleArn": "arn:aws:iam::${ACCOUNT_ID}:role/amtEcsTaskRole",
  "containerDefinitions": [
    {
      "name": "app",
      "image": "${ECR_REPO}:${TAG}",
      "essential": true,
      "portMappings": [
        { "containerPort": 8080, "protocol": "tcp" }
      ],
      "environment": [
        { "name": "PORT", "value": "8080" },
        { "name": "COOKIE_SECURE", "value": "false" }
      ],
      "secrets": [
        {
          "name": "DATABASE_URL",
          "valueFrom": "arn:aws:ssm:${AWS_REGION}:${ACCOUNT_ID}:parameter/amt/prod/DATABASE_URL"
        }
      ],
      "healthCheck": {
        "command": ["CMD-SHELL", "wget -q -O - http://localhost:8080/healthz || exit 1"],
        "interval": 30,
        "timeout": 5,
        "retries": 3,
        "startPeriod": 60
      },
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/anime-manga-tracker",
          "awslogs-region": "${AWS_REGION}",
          "awslogs-stream-prefix": "app"
        }
      }
    }
  ]
}
EOF

aws ecs register-task-definition --cli-input-json file:///tmp/amt-taskdef.json
```

設定の意味:

| 項目 | 値 | なぜ |
| --- | --- | --- |
| `cpu` / `memory` | 256 / 512 | Fargate の最小構成。Go + 静的ファイル配信には十分 |
| `COOKIE_SECURE` | `"false"` | Phase 1 は平文 HTTP。`true` にするとブラウザが Cookie を送らず**ログインできない** |
| `DATABASE_URL` | `secrets` 側 | `environment` に書くとタスク定義に平文で残る |
| `healthCheck` | `wget`（alpine の busybox 内蔵。`curl` はイメージに入っていない） | |
| `startPeriod: 60` | 起動時にマイグレーション適用があるため猶予を持たせる | |
| `cpuArchitecture` | `X86_64` | Step 5 の `--platform linux/amd64` と対になる |

> アプリは**起動時に未適用のマイグレーションを自動で流す**（`internal/infrastructure/postgres/migrate.go`）。
> 新規の空 RDS に対しては `0001`〜`0004` が順に適用される。別途マイグレーション作業は不要。

---

## Step 10: ECS サービスを作る

### 10-1. サブネットと SG の ID を集める

```bash
export SUBNET_PUB1=$(aws ec2 describe-subnets --filters "Name=tag:Name,Values=amt-subnet-public1-*" --query 'Subnets[0].SubnetId' --output text)
export SUBNET_PUB2=$(aws ec2 describe-subnets --filters "Name=tag:Name,Values=amt-subnet-public2-*" --query 'Subnets[0].SubnetId' --output text)
export TASK_SG=$(aws ec2 describe-security-groups --filters "Name=group-name,Values=amt-task-sg" --query 'SecurityGroups[0].GroupId' --output text)
echo $SUBNET_PUB1 $SUBNET_PUB2 $TASK_SG   # 3つとも値が出ることを確認
```

### 10-2. サービスを作成

```bash
aws ecs create-service \
  --cluster amt-cluster \
  --service-name amt-service \
  --task-definition anime-manga-tracker \
  --desired-count 1 \
  --capacity-provider-strategy capacityProvider=FARGATE_SPOT,weight=1 \
  --network-configuration "awsvpcConfiguration={subnets=[${SUBNET_PUB1},${SUBNET_PUB2}],securityGroups=[${TASK_SG}],assignPublicIp=ENABLED}" \
  --deployment-configuration "minimumHealthyPercent=0,maximumPercent=100,deploymentCircuitBreaker={enable=true,rollback=true}" \
  --enable-execute-command
```

各オプションの意味:

- **`minimumHealthyPercent=0` / `maximumPercent=100`**
  **この設定が今回いちばん重要。** 既定（100/200）だとローリングデプロイ中に新旧 2 タスクが
  並走する。このアプリはキャッシュ・single-flight・AniList のトークンバケットが**すべて
  プロセス内**にあるため、2 プロセスになると AniList への実効レートが倍になって 429 を踏むし、
  定期ジョブ（`home-cache-warm` / `next-airing-sync`）も二重に走る。
  数十秒のダウンタイムと引き換えに**常に 1 タスク**を保証する。
- **`assignPublicIp=ENABLED`** — NAT が無いので、これが無いと ECR からイメージを取ることすらできない。
- **`FARGATE_SPOT`** — 7 割前後安い代わりに、AWS 側の都合で**タスクが停止されることがある**
  （2 分前に SIGTERM が来て、サービスが自動で新しいタスクを立てる）。中断のたびに
  プロセス内キャッシュは空になり、パブリック IP も変わる。
- **`--enable-execute-command`** — ECS Exec の有効化。**サービス作成時に付けるのが楽**
  （後から付ける場合は `update-service` のうえで新しいデプロイを起こす必要がある）。
- **`deploymentCircuitBreaker`** — 新しいタスクが起動に失敗し続けたら自動で前のバージョンに戻す。
  壊れたイメージを push したときに延々とリトライし続けるのを止めてくれる。

### 10-3. 起動を待つ

```bash
aws ecs wait services-stable --cluster amt-cluster --services amt-service
```

2〜3 分で返る。**失敗する場合は「つまずきポイント」へ**。

---

## Step 11: 動作確認

### 11-1. パブリック IP を調べる

```bash
TASK_ARN=$(aws ecs list-tasks --cluster amt-cluster --service-name amt-service --query 'taskArns[0]' --output text)
ENI=$(aws ecs describe-tasks --cluster amt-cluster --tasks $TASK_ARN \
  --query 'tasks[0].attachments[0].details[?name==`networkInterfaceId`].value' --output text)
PUBLIC_IP=$(aws ec2 describe-network-interfaces --network-interface-ids $ENI \
  --query 'NetworkInterfaces[0].Association.PublicIp' --output text)
echo "http://${PUBLIC_IP}:8080"
```

> **この IP はデプロイ・再起動・Spot 中断のたびに変わる**。Fargate では Elastic IP を
> タスクに固定できない。固定アドレスが欲しくなった時点が Phase 2（ALB）に進むタイミング。

### 11-2. 確認する

```bash
curl -i http://${PUBLIC_IP}:8080/healthz
# HTTP/1.1 200 OK
# {"status":"ok"}
```

`{"status":"unhealthy"}` が返る場合は**アプリは動いていて DB に繋がっていない**。
`/healthz` は DB の `Ping` だけを見ているので、切り分けとしてはここで RDS 側の SG を疑う。

ブラウザで `http://<IP>:8080` を開き、

1. ホーム画面に今季アニメ・話題の漫画が出る（= AniList / Jikan への外向き通信が通っている）
2. サインアップ → ログインできる（= DB への書き込みとマイグレーションが通っている）
3. 作品を検索してライブラリに追加できる

ログを見る:

```bash
aws logs tail /ecs/anime-manga-tracker --follow
```

起動時に `applied migration ...`（初回のみ）と `listening on :8080` が出ていれば正常。
定期ジョブのログもここに流れる。

---

## 運用

### 新しいバージョンをデプロイする

```bash
export TAG=$(git rev-parse --short HEAD)

aws ecr get-login-password --region ${AWS_REGION} \
  | docker login --username AWS --password-stdin ${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com
docker build --platform linux/amd64 -t ${ECR_REPO}:${TAG} .
docker push ${ECR_REPO}:${TAG}

# タスク定義の image を差し替えて再登録
sed -i '' "s|${ECR_REPO}:[a-f0-9]*|${ECR_REPO}:${TAG}|" /tmp/amt-taskdef.json
aws ecs register-task-definition --cli-input-json file:///tmp/amt-taskdef.json

# サービスに反映
aws ecs update-service --cluster amt-cluster --service amt-service \
  --task-definition anime-manga-tracker
aws ecs wait services-stable --cluster amt-cluster --services amt-service
```

`minimumHealthyPercent=0` にしているので、**この間 30〜60 秒ほど落ちる**。意図した挙動。

### 実行中のコンテナに入る（ECS Exec）

```bash
# 事前に Session Manager plugin が必要（一度だけ）
brew install --cask session-manager-plugin

TASK_ARN=$(aws ecs list-tasks --cluster amt-cluster --service-name amt-service --query 'taskArns[0]' --output text)
aws ecs execute-command --cluster amt-cluster --task $TASK_ARN \
  --container app --interactive --command "/bin/sh"
```

> **このイメージには `psql` が入っていない**（`alpine:3.20` + バイナリだけ）。
> シェルには入れるが SQL は叩けない。DB を直接見たくなったら、その場で
> `apk add --no-cache postgresql-client` を実行する（タスクが入れ替わると消える）。
> 恒久的に必要になったら `Dockerfile` に入れるかを判断する。

### 費用を止める（しばらく触らないとき）

```bash
# ECS タスクを 0 にする（Fargate の課金が止まる）
aws ecs update-service --cluster amt-cluster --service amt-service --desired-count 0

# RDS を停止する
aws rds stop-db-instance --db-instance-identifier amt-db
```

> **RDS の「停止」は 7 日経つと自動で再起動する。** AWS の仕様。長期間放置するなら、
> スナップショットを取って**インスタンスごと削除**するのが確実。
>
> ```bash
> aws rds delete-db-instance --db-instance-identifier amt-db \
>   --final-db-snapshot-identifier amt-db-final-$(date +%Y%m%d)
> ```
>
> 停止中もストレージ（20GB）とスナップショットには課金される（わずか）。

再開:

```bash
aws rds start-db-instance --db-instance-identifier amt-db
# 「利用可能」になってから
aws ecs update-service --cluster amt-cluster --service amt-service --desired-count 1
```

### 全部消す

**この順番で消す**（依存があるので逆順だと消せない）。

```bash
aws ecs update-service --cluster amt-cluster --service amt-service --desired-count 0
aws ecs delete-service --cluster amt-cluster --service amt-service --force
aws ecs delete-cluster --cluster amt-cluster
aws rds delete-db-instance --db-instance-identifier amt-db --skip-final-snapshot --delete-automated-backups
aws ecr delete-repository --repository-name anime-manga-tracker --force
aws ssm delete-parameter --name /amt/prod/DATABASE_URL
aws logs delete-log-group --log-group-name /ecs/anime-manga-tracker
# IAM ロール 2 つ（インラインポリシーを消してから）とサブネットグループ・VPC はコンソールで
```

消し終わったら **請求ダッシュボードで翌日に $0 になっていることを確認する**。
VPC の削除は、中にリソースが残っていると失敗する（エラーメッセージが何が残っているかを教えてくれる）。

---

## つまずきポイント

症状から引ける表。**まず `aws logs tail /ecs/anime-manga-tracker --since 10m` と、
`aws ecs describe-tasks` の `stoppedReason` を見ること。**

```bash
# 止まったタスクの理由を見る
aws ecs describe-tasks --cluster amt-cluster \
  --tasks $(aws ecs list-tasks --cluster amt-cluster --desired-status STOPPED --query 'taskArns[0]' --output text) \
  --query 'tasks[0].{stopped:stoppedReason,containers:containers[].{name:name,reason:reason,exit:exitCode}}'
```

| 症状 | 原因 | 対処 |
| --- | --- | --- |
| タスクが起動と停止を繰り返す / ログに `exec format error` | arm64 でビルドした | `docker build --platform linux/amd64` で再ビルドして push |
| `ResourceInitializationError: unable to pull secrets or registry auth` | ① パブリック IP が OFF ② サブネットがプライベート ③ 実行ロールに `ssm:GetParameters` が無い | ①② はサービスのネットワーク設定、③ は Step 6-1 |
| `CannotPullContainerError` | 同上（外に出られていない） | `assignPublicIp=ENABLED` とパブリックサブネットを確認 |
| ログに `ping db: ... context deadline exceeded` でタスクが落ちる | RDS の SG がタスク SG からの 5432 を許可していない | Step 2-2。**IP ではなく SG を指定**しているか |
| ログに `pg_hba.conf entry ... SSL off` | `DATABASE_URL` に `sslmode=require` が無い | SSM のパラメータを修正 → タスクを再起動 |
| ログに `database "anime_manga_tracker" does not exist` | RDS 作成時に「最初のデータベース名」を入れ忘れた | ECS Exec で `psql` を入れて `CREATE DATABASE`、または RDS を作り直す |
| ログに `DATABASE_URL is required` | `secrets` の `valueFrom` の ARN が違う / パラメータ名のタイポ | ARN を `aws ssm get-parameter --name /amt/prod/DATABASE_URL` で確認 |
| `/healthz` が `{"status":"unhealthy"}` | アプリは動いていて DB だけ繋がらない | RDS が「利用可能」か、SG、`sslmode` |
| ブラウザから繋がらない（タスクは RUNNING） | ① 自宅 IP が変わった ② ポート 8080 を開けていない ③ IP が古い | `amt-task-sg` のインバウンドを「マイ IP」で更新 / Step 11-1 で IP を取り直す |
| 画面は出るがログインできない | `COOKIE_SECURE=true` になっている（平文 HTTP では Cookie が送られない） | タスク定義の `environment` を `"false"` に |
| ホーム画面が空 / 作品が出ない | タスクが外に出られていない（AniList/Jikan に届かない） | SG の**アウトバウンド**を絞っていないか |
| ECS Exec が `TargetNotConnectedException` | ① タスクロールに `ssmmessages` が無い ② `--enable-execute-command` 後にデプロイしていない | Step 6-2 / `aws ecs update-service --force-new-deployment` |
| ECS Exec が `SessionManagerPlugin is not found` | プラグイン未インストール | `brew install --cask session-manager-plugin` |
| サービス作成で `FARGATE_SPOT ... not found` | クラスタに capacity provider が登録されていない | Step 8 の `--capacity-providers` |
| デプロイしたのに古いコードが動いている | タグが `latest` のまま / タスク定義を再登録していない | コミット SHA タグを使う |

---

## Phase 2: HTTPS で公開する（この手順書の範囲外・道筋のみ）

Phase 1 は**平文 HTTP で IP 直打ち**なので、常用してはいけない。公開するなら:

1. **ドメインを取得**（Route 53 で取るか、持っているものを移管）
2. **ACM で証明書を発行**（Route 53 の DNS 検証なら自動。ALB と同じリージョンで取る）
3. **ターゲットグループ**を作る（ターゲットタイプ **IP**、ポート 8080、ヘルスチェックパス `/healthz`）
4. **ALB** を作る（パブリックサブネット 2 つ、`amt-alb-sg` はインバウンド 80/443 を全世界に）
5. **タスク SG のインバウンドを `amt-alb-sg` からの 8080 のみに変更**（自分の IP のルールは削除）
6. **ECS サービスにターゲットグループを紐付ける**
   （**既存サービスには後から付けられない**。サービスを作り直す）
7. **HTTP → HTTPS のリダイレクト**をリスナーに設定
8. **`COOKIE_SECURE=true` に変更**してタスク定義を再登録（Phase 1 で `false` のままだと
   HTTPS 化の意味が半減する。`internal/config/config.go` の既定は `false`）

追加費用は **ALB 月 $18 前後**。Phase 1 の $20 と合わせて月 $38 前後。

---

## この後: Terraform 化

`BACKLOG.md` に書いたとおり、**ここまでを手で一周したら全部削除して Terraform で作り直す**。
手で作る過程で「クラスタ・サービス・タスク定義・SG の関係」が分かっているはずで、それが無いまま
IaC を書くとエラーの意味が読めない。

Terraform 化の実利は主に 2 つ:

- **`terraform destroy` で確実に全部消せる**（消し忘れによる課金が止まる）。手作業だと
  上の「全部消す」のような手順を毎回踏むことになり、ENI や SG が残りやすい
- GitHub Actions からの自動デプロイに繋げられる。認証は **OIDC で AssumeRole**
  （アクセスキーを GitHub Secrets に置かない）。現在の `.github/workflows/ci.yml` は
  build までで、デプロイは未接続

作り直すときの単位は、この手順書の Step 1〜10 がそのままリソースの単位になる。
