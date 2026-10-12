# AWS デプロイ手順書（Phase 2: ALB + HTTPS 公開）

**[Phase 1](DEPLOY-AWS.md) が動いている状態から続ける。** Phase 1 は「平文 HTTP・IP 直打ち・
自分の IP だけに開ける」学習用の足場だった。ここで ALB・証明書・独自ドメインを足して、
**常用できる構成にする**のがゴール。

構成図は **[`diagrams/aws-architecture.drawio`](diagrams/aws-architecture.drawio)** の
**Phase 2** ページ（開き方は [`diagrams/README.md`](diagrams/README.md)）。

最終更新: 2026-10-13

---

## 0. この作業で作るもの

### Phase 1 からの差分

| | Phase 1 | Phase 2 |
| --- | --- | --- |
| アクセス先 | `http://<タスクのパブリック IP>:8080`（毎回変わる） | `https://<独自ドメイン>`（固定） |
| 暗号化 | なし（平文 HTTP） | ALB で TLS 終端 |
| タスク SG のインバウンド | 自分の IP → 8080 | **ALB SG → 8080 のみ** |
| `COOKIE_SECURE` | `false` | **`true`** |
| `TRUST_PROXY` | `false` | **`true`** |
| 追加リソース | — | ACM 証明書 / ALB / ターゲットグループ / Route 53 ホストゾーン |
| 月額 | $20 前後 | **$38〜40 前後** |

**変えないもの**（Phase 1 の判断を引き継ぐ）:

- **NAT ゲートウェイは作らない。** タスクはパブリックサブネットに置いたまま、
  `assignPublicIp=ENABLED` も維持する。SG を閉じるのでインターネットから直接は叩けなくなるが、
  ECR・SSM・CloudWatch・AniList への**外向き通信にはパブリック IP が要る**。
- **タスクは常に 1 つ。** `desiredCount: 1` / `minimumHealthyPercent: 0` / `maximumPercent: 100` のまま。
  → **ALB を入れてもゼロダウンタイムにはならない**（後述）。

### 作業の順番と依存関係

| Step | 作るもの | 次に何で使うか |
| --- | --- | --- |
| 1 | ドメイン（Route 53）+ ホストゾーン | 証明書・レコード |
| 2 | ACM 証明書 | ALB のリスナー |
| 3 | `amt-alb-sg` | ALB・タスク SG |
| 4 | ターゲットグループ `amt-tg` | ALB・ECS サービス |
| 5 | ALB + リスナー（443 / 80 リダイレクト） | Route 53 レコード |
| 6 | Route 53 の A レコード（Alias） | 動作確認 |
| 7 | ECS サービスに TG を紐付け | 動作確認 |
| 8 | タスク SG を ALB からのみに変更 | — |
| 9 | `COOKIE_SECURE=true` + `TRUST_PROXY=true` | — |
| 10 | 動作確認 | — |
| 11 | サインアップを閉じる（BACKLOG 4-b） | — |

**Step 8 は Step 7 の確認が済んでから。** 先に自分の IP を塞ぐと、ALB 経由が繋がらなかったときに
確認する手段が無くなる。

**Step 11 は Step 10 の後。** 先に閉じると、動作確認でテスト用アカウントを作れなくなる。

### 所要時間とお金

- 所要時間: **2〜3 時間**（ドメイン登録の反映とACM の検証で待つ時間を含む）
- 追加月額:

| 項目 | 金額 | 備考 |
| --- | --- | --- |
| ALB | **月 $18 前後** | + LCU 従量課金。この規模ではほぼゼロ |
| Route 53 ホストゾーン | **月 $0.50** | ドメイン購入時に自動で 1 つ作られる |
| ドメイン登録 | **年 $14 前後**（.com） | TLD による。**返金されない** |
| ACM 証明書 | **無料** | ALB で使う限り |

Phase 1 の $20 と合わせて **月 $38〜40 前後**。

> **ALB は「停止」できない。** RDS や ECS タスクと違い、使わない間だけ止める方法が無い。
> 放置すると月 $18 が積み上がる。長期間触らないなら ALB ごと削除すること（「運用」に手順）。

### 作業中ずっと使う環境変数

Phase 1 のものに加えて使う。ターミナルを開き直すたびに流す。

```bash
export AWS_REGION=ap-northeast-1
export AWS_PAGER=""
export ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

export VPC_ID=$(aws ec2 describe-vpcs --filters "Name=tag:Name,Values=amt-vpc" --query 'Vpcs[0].VpcId' --output text)
export SUBNET_PUB1=$(aws ec2 describe-subnets --filters "Name=tag:Name,Values=amt-subnet-public1-*" --query 'Subnets[0].SubnetId' --output text)
export SUBNET_PUB2=$(aws ec2 describe-subnets --filters "Name=tag:Name,Values=amt-subnet-public2-*" --query 'Subnets[0].SubnetId' --output text)
export TASK_SG=$(aws ec2 describe-security-groups --filters "Name=group-name,Values=amt-task-sg" --query 'SecurityGroups[0].GroupId' --output text)

echo "VPC=$VPC_ID  PUB1=$SUBNET_PUB1  PUB2=$SUBNET_PUB2  TASK_SG=$TASK_SG"
```

`DOMAIN` / `CERT_ARN` / `ALB_SG` / `TG_ARN` / `ALB_ARN` / `ZONE_ID` は各 Step で設定する。

---

## Step 1: ドメインを取得する（Route 53）

**ここで年額の課金が発生し、取り消せない。** 金額を確認してから確定すること。

1. **Route 53** コンソール → 左メニュー **ドメイン** → **登録**
2. 欲しいドメイン名を検索する
   - 学習用なので TLD は何でもよい。`.com` が年 $14 前後、`.net` が $13 前後。
     `.click` `.link` などは年 $3〜5 と安い（ACM の証明書はどの TLD でも同じように発行できる）
   - 価格は検索結果に表示される。**表示額が年額**
3. カートに追加 → 連絡先情報を入力
   - **プライバシー保護は既定でオン**のままにする（無料。Whois に自宅住所が出るのを防ぐ）
4. **自動更新（Auto-renew）** — 既定で**オン**
   - 学習が終わったら忘れずにオフにする（「運用」参照）。オンのままだと翌年も課金される
5. 規約に同意して注文を確定

### 1-1. 登録者メールアドレスの検証（必須・忘れると止まる）

注文後、登録者メールアドレス宛に **ICANN の検証メール**が届く。

> **このメールのリンクを踏まないと、15 日後にドメインが停止される。**
> ドメインが停止すると名前解決ができなくなり、サイトに繋がらなくなる。すぐに踏むこと。

### 1-2. 完了を待つ

登録完了まで**数分〜数時間**。完了すると:

- Route 53 → ドメイン → 「登録済みドメイン」に出る
- **ホストゾーンが自動で作られる**（ここから月 $0.50）

```bash
export DOMAIN=example.com   # 取得したドメインに置き換える
export ZONE_ID=$(aws route53 list-hosted-zones-by-name --dns-name "$DOMAIN" \
  --query "HostedZones[?Name=='${DOMAIN}.'].Id | [0]" --output text | sed 's|/hostedzone/||')
echo "DOMAIN=$DOMAIN  ZONE_ID=$ZONE_ID"
```

`ZONE_ID` が `None` なら、まだ登録が完了していないか、ドメイン名が違う。

---

## Step 2: ACM で証明書を発行する

1. **Certificate Manager** コンソールを開く
   - **右上のリージョンが `アジアパシフィック (東京) ap-northeast-1` であることを確認する。**
     ACM の証明書は**リージョンをまたいで使えない**。ALB と同じリージョンで取る必要がある
     （`us-east-1` で取るのは CloudFront 用。今回は関係ない）
2. **証明書をリクエスト** → **パブリック証明書をリクエスト**
3. ドメイン名:
   - 1 つめ: `example.com`（取得したドメインそのもの）
   - **別の名前を追加** → 2 つめ: `*.example.com`
     - ワイルドカードを入れておくと、あとから `api.example.com` などを足すときに取り直さずに済む。
       **追加料金はない**
4. 検証方法: **DNS 検証 - 推奨**
5. キーアルゴリズム: `RSA 2048`（既定）
6. **リクエスト**

### 2-1. DNS 検証レコードを入れる

1. 発行された証明書の詳細画面を開く（ステータスは「**検証保留中**」）
2. 「ドメイン」セクションの **Route 53 でレコードを作成** ボタンを押す
3. 2 つのドメイン名にチェックが入っていることを確認 → **レコードを作成**

> **Route 53 でドメインを取ったのでこのボタンが使える。** 他社で取ったドメインの場合は
> 表示される CNAME を自分で DNS に登録することになる。

### 2-2. 発行を待つ

数分〜最大 30 分。CLI で待てる:

```bash
export CERT_ARN=$(aws acm list-certificates --region ${AWS_REGION} \
  --query "CertificateSummaryList[?DomainName=='${DOMAIN}'].CertificateArn | [0]" --output text)
echo $CERT_ARN

aws acm wait certificate-validated --certificate-arn $CERT_ARN --region ${AWS_REGION}
echo "発行完了"
```

ステータスが「**発行済み**」になれば次へ。

---

## Step 3: ALB 用セキュリティグループを作る

```bash
export ALB_SG=$(aws ec2 create-security-group \
  --group-name amt-alb-sg \
  --description "ALB for anime-manga-tracker" \
  --vpc-id $VPC_ID \
  --query 'GroupId' --output text)

aws ec2 authorize-security-group-ingress --group-id $ALB_SG --protocol tcp --port 80 --cidr 0.0.0.0/0
aws ec2 authorize-security-group-ingress --group-id $ALB_SG --protocol tcp --port 443 --cidr 0.0.0.0/0

echo "ALB_SG=$ALB_SG"
```

> **ここは全世界に開ける。** Phase 1 でタスク SG を自分の IP に絞っていたのは、平文 HTTP を
> 晒さないためだった。Phase 2 は HTTPS で公開する構成なので、ALB の 80/443 は誰でも
> 叩ける状態が正しい。守りはこの先（タスク SG・RDS SG）で効かせる。
>
> 80 を開けるのは **HTTPS にリダイレクトするため**。閉じると `http://` で来た人が
> エラーになるだけで、セキュリティ上の利点はない。

---

## Step 4: ターゲットグループを作る

ALB が「どこに流すか」の定義。**ALB より先に作る**（リスナーが参照するため）。

1. **EC2** コンソール → 左メニュー **ターゲットグループ** → **ターゲットグループの作成**
2. 設定:

| 項目 | 値 | 注意 |
| --- | --- | --- |
| ターゲットタイプ | **IP アドレス** | **`インスタンス` では作れない。** Fargate は `awsvpc` モードで ENI を持つため IP ターゲットになる。ここを間違えると ECS が登録できず作り直し |
| ターゲットグループ名 | `amt-tg` | |
| プロトコル : ポート | **HTTP : 8080** | ALB → タスクの間は平文。TLS は ALB で終端する |
| IP アドレスタイプ | IPv4 | |
| VPC | **amt-vpc** | |
| プロトコルバージョン | HTTP1 | |

3. **ヘルスチェック**:

| 項目 | 値 | 理由 |
| --- | --- | --- |
| プロトコル | HTTP | |
| **パス** | **`/healthz`** | `internal/handler/health_handler.go`。DB の `Ping` までを見る |
| ポート（詳細設定） | トラフィックポート | = 8080 |
| 正常のしきい値 | 2 | |
| 非正常のしきい値 | 2 | |
| タイムアウト | 5 秒 | `/healthz` 側の上限は 2 秒なので、先にこちらが切れることはない |
| 間隔 | 30 秒 | |
| 成功コード | 200 | DB が落ちると `/healthz` は 503 を返すので、自動で切り離される |

4. 次へ → **ターゲットは登録しない**（ECS サービスが自動で登録する）→ 作成

### 4-1. 登録解除の遅延を 30 秒に下げる（重要）

```bash
export TG_ARN=$(aws elbv2 describe-target-groups --names amt-tg \
  --query 'TargetGroups[0].TargetGroupArn' --output text)

aws elbv2 modify-target-group-attributes --target-group-arn $TG_ARN \
  --attributes Key=deregistration_delay.timeout_seconds,Value=30

echo "TG_ARN=$TG_ARN"
```

> **既定は 300 秒（5 分）。** 旧タスクを切り離すときに「処理中のリクエストが終わるのを待つ」
> 時間で、下げないと**デプロイのたびに 5 分待たされる**。このアプリは数秒で終わる
> リクエストしか無いので 30 秒で足りる。

---

## Step 5: ALB を作る

1. **EC2** コンソール → **ロードバランサー** → **ロードバランサーの作成**
2. **Application Load Balancer** を選択
3. 基本設定:

| 項目 | 値 | 注意 |
| --- | --- | --- |
| 名前 | `amt-alb` | |
| スキーム | **インターネット向け** | 内部にするとインターネットから届かない |
| IP アドレスタイプ | IPv4 | |

4. ネットワークマッピング:
   - VPC: **amt-vpc**
   - マッピング: **パブリックサブネットを 2 つとも**チェック
     - **ALB は 2 つ以上の AZ が必須。** プライベートサブネットを選ぶとインターネットから届かない
5. セキュリティグループ: **`amt-alb-sg` のみ**（`default` が入っていたら**外す**）
6. リスナーとルーティング:
   - プロトコル **HTTPS** / ポート **443** / デフォルトアクション: **`amt-tg` に転送**
7. セキュアリスナーの設定:
   - セキュリティポリシー: `ELBSecurityPolicy-TLS13-1-2-2021-06`（既定でよい）
   - 証明書ソース: **ACM から** → Step 2 の証明書を選ぶ
8. **ロードバランサーの作成**

プロビジョニング完了まで 2〜3 分。

### 5-1. HTTP(80) → HTTPS リダイレクトのリスナーを足す

```bash
export ALB_ARN=$(aws elbv2 describe-load-balancers --names amt-alb \
  --query 'LoadBalancers[0].LoadBalancerArn' --output text)

aws elbv2 create-listener \
  --load-balancer-arn $ALB_ARN \
  --protocol HTTP --port 80 \
  --default-actions '[{
    "Type": "redirect",
    "RedirectConfig": {
      "Protocol": "HTTPS",
      "Port": "443",
      "StatusCode": "HTTP_301"
    }
  }]'
```

- **301（恒久的）**にする。ブラウザがリダイレクトを覚えるので、2 回目以降は HTTP を叩かなくなる
- ホスト名とパスは元のまま引き継がれる（`#{host}` `#{path}` が既定）

ALB の DNS 名を控える:

```bash
aws elbv2 describe-load-balancers --names amt-alb \
  --query 'LoadBalancers[0].DNSName' --output text
# amt-alb-123456789.ap-northeast-1.elb.amazonaws.com
```

---

## Step 6: Route 53 に A レコード（Alias）を作る

1. **Route 53** → **ホストゾーン** → 取得したドメインを開く
2. **レコードを作成**
3. 設定:

| 項目 | 値 |
| --- | --- |
| レコード名 | **空のまま**（= `example.com` そのもの） |
| レコードタイプ | **A** |
| **エイリアス** | **オン** |
| トラフィックのルーティング先 | **Application/Classic Load Balancer へのエイリアス** → **アジアパシフィック (東京)** → `amt-alb-...` |
| ルーティングポリシー | シンプルルーティング |

4. **レコードを作成**
5. `www` でもアクセスしたいなら、レコード名を `www` にして同じものをもう 1 本作る

> **なぜ A の「エイリアス」なのか。** ALB の IP アドレスは AWS 側で勝手に変わるので、
> A レコードに IP を直書きできない。かといって CNAME は仕様上
> **ドメインの頂点（`example.com` そのもの）には設定できない**。
> Alias は Route 53 独自の仕組みで、頂点に使えて、しかも**クエリ課金が無料**になる。

反映の確認（数十秒〜数分）:

```bash
dig +short $DOMAIN
# ALB の IP が 2 つ前後返ればOK
```

---

## Step 7: ECS サービスをターゲットグループに紐付ける

```bash
aws ecs update-service \
  --cluster amt-cluster \
  --service amt-service \
  --load-balancers targetGroupArn=$TG_ARN,containerName=app,containerPort=8080 \
  --health-check-grace-period-seconds 120

aws ecs wait services-stable --cluster amt-cluster --services amt-service
```

- **`containerName` はタスク定義のコンテナ名** (`app`)。違うと紐付けに失敗する
- **`--health-check-grace-period-seconds 120` は必須級。**
  これが無いと、タスク起動直後（マイグレーション適用中でまだ listen していない時間）に
  ALB のヘルスチェックが落ちて ECS がタスクを殺し、**起動と停止を延々繰り返すループ**になる。
  猶予時間の間はヘルスチェックの失敗を無視する

> ローリング更新（ECS デプロイコントローラ）のサービスなら、ロードバランサー構成は
> **後から変更できる**。もしこのコマンドが拒否された場合は、サービスを作り直す
> （[付録 A](#付録-a-サービスを作り直す場合)）。

### 7-1. ターゲットが healthy になるのを確認する

```bash
aws elbv2 describe-target-health --target-group-arn $TG_ARN \
  --query 'TargetHealthDescriptions[].{IP:Target.Id,Port:Target.Port,State:TargetHealth.State,Reason:TargetHealth.Reason}' \
  --output table
```

`State` が `healthy` になるまで 1〜2 分。`unhealthy` のままなら「つまずきポイント」へ。

**この時点ではまだ繋がらない。** タスク SG が ALB からの 8080 を許可していないため。次の Step で開ける。

---

## Step 8: タスク SG のインバウンドを ALB からのみにする

**順番が大事。** 先に ALB からの許可を足し、ALB 経由で繋がることを確認してから、自分の IP を消す。

### 8-1. ALB SG からの 8080 を許可する

```bash
aws ec2 authorize-security-group-ingress \
  --group-id $TASK_SG --protocol tcp --port 8080 --source-group $ALB_SG
```

1〜2 分待ってターゲットが `healthy` になったら、**ブラウザで `https://<ドメイン>` を開いて
表示されることを確認する**。

### 8-2. 自分の IP のルールを消す

繋がることを確認してから。いま入っているルールを見る:

```bash
aws ec2 describe-security-groups --group-ids $TASK_SG \
  --query 'SecurityGroups[0].IpPermissions[].{Port:FromPort,Cidr:IpRanges[].CidrIp,SG:UserIdGroupPairs[].GroupId}'
```

`Cidr` に出てくる自分の IP のルールを削除する:

```bash
aws ec2 revoke-security-group-ingress \
  --group-id $TASK_SG --protocol tcp --port 8080 --cidr 203.0.113.45/32   # ← 実際の値に置き換え
```

> **タスクのパブリック IP は外さない。** SG を閉じたのでインターネットからは叩けなくなるが、
> NAT が無い以上、ECR・SSM・CloudWatch・AniList への**外向き通信にはパブリック IP が必要**。
> `assignPublicIp=ENABLED` は維持する。

---

## Step 9: COOKIE_SECURE と TRUST_PROXY を true にする

**2 つとも同じタイミングで切り替える。** どちらも「ALB が前に入った」ことで正解が変わる設定で、
Step 8 で SG を ALB からのみに閉じた今がその境目にあたる。

| 変数 | なぜ切り替えるか | 忘れると |
| --- | --- | --- |
| `COOKIE_SECURE` | HTTPS になったので Cookie に `Secure` を立てる | Cookie が平文 HTTP でも送られる状態のまま。HTTPS 化の効果が半減する |
| `TRUST_PROXY` | クライアント IP を `X-Forwarded-For` から取る | **全利用者が 1 つのレート制限バケットを共有し、数人が普通に使っただけで 429 になる** |

`TRUST_PROXY` のほうは**サイトが使えなくなる**ので、取りこぼすと痛い。理屈はこう:

- Step 8 でタスク SG を ALB SG からのみに閉じたため、`RemoteAddr` は**常に ALB** になった
- `TRUST_PROXY=false` のままだと、レート制限のキーが全員 ALB の IP になる
- 逆に Phase 1 で `true` にすると、クライアントが `X-Forwarded-For` を自分で書くだけで
  レート制限を回避できてしまう。**どちらの値も、前段の構成と対でしか正しくない**

詳細は `internal/config/config.go` の `TrustProxy` と `internal/handler/router.go` の
`IPExtractor` のコメントにある。

```bash
# タスク定義の COOKIE_SECURE と TRUST_PROXY を false -> true に
sed -i '' 's/{ "name": "COOKIE_SECURE", "value": "false" }/{ "name": "COOKIE_SECURE", "value": "true" }/' /tmp/amt-taskdef.json
sed -i '' 's/{ "name": "TRUST_PROXY", "value": "false" }/{ "name": "TRUST_PROXY", "value": "true" }/' /tmp/amt-taskdef.json

# 2 つとも true になっているか確認（TRUST_PROXY が無ければ environment に足す）
grep -E 'COOKIE_SECURE|TRUST_PROXY' /tmp/amt-taskdef.json

aws ecs register-task-definition --cli-input-json file:///tmp/amt-taskdef.json

aws ecs update-service --cluster amt-cluster --service amt-service \
  --task-definition anime-manga-tracker
aws ecs wait services-stable --cluster amt-cluster --services amt-service
```

> `/tmp/amt-taskdef.json` が残っていない場合は、現在のタスク定義を落として編集する:
>
> ```bash
> aws ecs describe-task-definition --task-definition anime-manga-tracker \
>   --query 'taskDefinition' --output json \
>   | jq 'del(.taskDefinitionArn,.revision,.status,.requiresAttributes,.compatibilities,.registeredAt,.registeredBy)' \
>   > /tmp/amt-taskdef.json
> ```

> **切り替え後、一度ログインし直すことになる。**
> `internal/handler/auth_handler.go` のコメントにあるとおり、ブラウザは `Secure` 属性が
> 食い違う Cookie を**別物として扱う**。Phase 1 で発行された `Secure` なしの Cookie が残っていると
> 上書きできず、挙動が不安定になる。ブラウザの Cookie を消すか、シークレットウィンドウで確認する。

---

## Step 10: 動作確認

### 10-1. リダイレクトと証明書

```bash
# HTTP -> HTTPS の 301
curl -sI http://${DOMAIN} | head -3
# HTTP/1.1 301 Moved Permanently
# Location: https://example.com:443/

# HTTPS で 200
curl -s https://${DOMAIN}/healthz
# {"status":"ok"}

# 証明書の中身
echo | openssl s_client -connect ${DOMAIN}:443 -servername ${DOMAIN} 2>/dev/null \
  | openssl x509 -noout -subject -issuer -dates
```

### 10-2. 旧経路が塞がったことの確認

```bash
# タスクのパブリック IP を取る
TASK_ARN=$(aws ecs list-tasks --cluster amt-cluster --service-name amt-service --query 'taskArns[0]' --output text)
ENI=$(aws ecs describe-tasks --cluster amt-cluster --tasks $TASK_ARN \
  --query 'tasks[0].attachments[0].details[?name==`networkInterfaceId`].value' --output text)
PUBLIC_IP=$(aws ec2 describe-network-interfaces --network-interface-ids $ENI \
  --query 'NetworkInterfaces[0].Association.PublicIp' --output text)

# 直接は繋がらないはず（タイムアウトすればOK）
curl --max-time 5 http://${PUBLIC_IP}:8080/healthz
# curl: (28) Connection timed out  ← これが正しい
```

### 10-3. ブラウザで

1. `https://<ドメイン>` を開く → **鍵マークが出る**
2. **Cookie を一度消してから**サインアップ / ログイン
3. 開発者ツール → Application → Cookies で、セッション Cookie に **`Secure` と `HttpOnly` が付いている**
4. ホーム画面に今季アニメ・話題の漫画が出る（外向き通信が生きている）
5. 検索 → ライブラリに追加（DB 書き込み）

> **ここが自由にサインアップできる最後の機会。** Step 11 でサインアップを閉じるので、
> 常用するアカウントはこの段階で作っておく（閉じた後に作る手順は 11-3 にある）。

### 10-4. レート制限が IP ごとに効いているか（TRUST_PROXY の検証）

**ここを飛ばさないこと。** `TRUST_PROXY` を取りこぼしていると、全利用者が ALB の IP で
1 つのバケットを共有する。自分 1 人で触っている間は気付けず、人が増えてから 429 で露見する。

```bash
# 未認証 API を 40 回連続で叩く（バースト 30 / 毎秒 2 本なので途中で 429 に入る）
for i in $(seq 1 40); do
  curl -s -o /dev/null -w '%{http_code} ' "https://${DOMAIN}/api/home/trending"
done; echo
# 200 が 30 強、そのあと 429 が混じる ← バケットが効いている
```

**判定はここから。** 上で 429 を出したすぐあとに、**別の回線**（スマホのモバイル通信、
テザリングを切った状態）から 1 本だけ叩く。

```bash
curl -s -o /dev/null -w '%{http_code}\n' "https://${DOMAIN}/api/home/trending"
```

| 結果 | 意味 |
| --- | --- |
| `200` | **正しい。** IP ごとに別バケットになっている |
| `429` | **設定ミス。** 全員が同じバケットを共有している |

`429` が返ったら `TRUST_PROXY` がタスク定義に入っていないか `false` のまま。
Step 9 に戻り、タスク定義の `environment` を確認する。

```bash
aws ecs describe-task-definition --task-definition anime-manga-tracker \
  --query 'taskDefinition.containerDefinitions[0].environment' --output table
```

### 10-5. ターゲットとログ

```bash
aws elbv2 describe-target-health --target-group-arn $TG_ARN \
  --query 'TargetHealthDescriptions[].TargetHealth.State' --output text
# healthy

aws logs tail /ecs/anime-manga-tracker --since 10m
```

**ついでに `DATABASE_URL` がログに出ていないか確認する**（BACKLOG 4-d）。タスク定義を触った
今が一番やりやすい。わざと壊した値で 1 回だけ起動し、CloudWatch に接続文字列が出ないか見る。
pgx のパースエラーに接続文字列が含まれるケースがあるため。確認したらすぐ元に戻すこと。

> ヘルスチェックはアプリのログには出ない（このアプリはリクエストログのミドルウェアを
> 入れていないため）。30 秒ごとのアクセスでログが溢れる心配はない。

---

## Step 11: サインアップを閉じる（BACKLOG 4-b）

**動作確認が終わってから最後にやる。** 順番を守ること——先に閉じると Step 10-3 で
テスト用アカウントを作れなくなる。**自分のアカウントを作り終えてから**この手順に進む。

### なぜ閉じるか

誰でもアカウントを作れる状態で公開すると、設計の前提が 2 つ崩れる。

- **`next-airing-sync` の AniList 消費がユーザー数に比例して増える。** このジョブは
  「処理すべき件数が 0 に収束する」ことを前提に 30 分間隔・100 ID に設定してある
  （BACKLOG 2-b）。登録作品が増え続けると収束しなくなる
- **RDS の 20GB を埋められる**

学習用の個人アプリなので、**閉じるのが一番安い。** アプリ側の変更は要らない。

### 11-1. リスナールールを足す

```bash
export LISTENER_ARN=$(aws elbv2 describe-listeners --load-balancer-arn $ALB_ARN \
  --query "Listeners[?Port==\`443\`].ListenerArn | [0]" --output text)

aws elbv2 create-rule \
  --listener-arn $LISTENER_ARN \
  --priority 10 \
  --conditions '[{"Field":"path-pattern","PathPatternConfig":{"Values":["/api/auth/signup"]}}]' \
  --actions '[{"Type":"fixed-response","FixedResponseConfig":{"StatusCode":"403","ContentType":"application/json","MessageBody":"{\"error\":\"signup is closed\"}"}}]'
```

> **優先度は小さいほど先に評価される。** 既定アクション（タスクへ転送）より前に効かせる
> 必要があるので、他のルールがなければ `10` でよい。

### 11-2. 閉じたことを確認する

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST "https://${DOMAIN}/api/auth/signup" \
  -H 'Content-Type: application/json' \
  -d '{"email":"blocked@example.com","password":"password123"}'
# 403  ← これが正しい

# ログインは今までどおり通ること（閉じすぎていないか）
curl -s -o /dev/null -w '%{http_code}\n' -X POST "https://${DOMAIN}/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"<自分のメール>","password":"<自分のパスワード>"}'
# 200
```

ブラウザでも、既にログイン済みのセッションがそのまま使えることを確認する。

> **フロントの登録画面は残る。** `/signup` を開けてフォームも出るが、送信すると 403 になる。
> 画面から導線を消すのは別タスク（UI 変更が要る）。自分しか使わない前提なら放置でよい。

### 11-3. 自分のアカウントを追加したくなったら

ルールを一時的に消して作り、また足す。

```bash
# ルール ARN を控えてから消す
aws elbv2 describe-rules --listener-arn $LISTENER_ARN \
  --query 'Rules[?Priority==`10`].RuleArn' --output text

aws elbv2 delete-rule --rule-arn <上で出た ARN>
# ... サインアップ ...
# 11-1 のコマンドでもう一度足す
```

---

## 運用（Phase 1 からの差分）

### デプロイ

手順は [Phase 1 の「新しいバージョンをデプロイする」](DEPLOY-AWS.md#新しいバージョンをデプロイする)
と同じ。ALB を入れても**ダウンタイムは残る**。

> **ALB を入れてもゼロダウンタイムにはならない。**
> `minimumHealthyPercent: 0` / `maximumPercent: 100` を維持しているので、デプロイ中は
> 旧タスクが止まってから新タスクが立つ。この間（30〜60 秒）ALB は healthy なターゲットを
> 失い、**503 を返す**。
>
> これは設定ミスではなく意図した制約で、キャッシュ・single-flight・AniList のトークンバケットが
> すべてプロセス内にあるため 2 タスクを並走させられないことに由来する
> （`BACKLOG.md` の「3. AWS へのデプロイ」）。ゼロダウンタイムにしたいなら、
> **先に ElastiCache へ状態を出す**のが順番。

### 費用を止める

```bash
aws ecs update-service --cluster amt-cluster --service amt-service --desired-count 0
aws rds stop-db-instance --db-instance-identifier amt-db
```

> **これでは ALB の月 $18 は止まらない。** ALB に停止の概念は無く、存在するだけで課金される。
> 数週間以上触らないなら ALB ごと削除する:
>
> ```bash
> aws elbv2 delete-load-balancer --load-balancer-arn $ALB_ARN
> # Route 53 の A レコードも参照先が消えるので削除する（コンソールから）
> ```
>
> ターゲットグループ・証明書・ホストゾーンは残しておいてよい（TG と ACM は無料、
> ホストゾーンは月 $0.50）。ALB を作り直したら Step 5-1・6・7 をやり直す。

### ドメインを手放す

**ドメイン登録は途中解約できない。** 期限まで有効で、返金もされない。できるのは
**自動更新を止めること**だけ。

```bash
# 自動更新をオフにする（us-east-1 固定。Route 53 Domains は us-east-1 のみ）
aws route53domains disable-domain-auto-renew --domain-name $DOMAIN --region us-east-1

# 確認
aws route53domains get-domain-detail --domain-name $DOMAIN --region us-east-1 \
  --query 'AutoRenew'
# false
```

> **学習が一段落したら、まずこれをやる。** オンのままだと翌年も自動で課金される。
> ホストゾーンを消しても、ドメイン登録の自動更新は止まらない（別物）。

### Phase 2 ぶんを全部消す

Phase 1 の「全部消す」の**前に**この順番で消す。

```bash
# 1. ALB（リスナーも一緒に消える）
aws elbv2 delete-load-balancer --load-balancer-arn $ALB_ARN
sleep 30

# 2. ターゲットグループ（ALB を消してから）
aws elbv2 delete-target-group --target-group-arn $TG_ARN

# 3. ALB の SG（ALB が完全に消えてから。ENI が残っていると失敗する）
aws ec2 delete-security-group --group-id $ALB_SG

# 4. ACM 証明書（ALB に紐付いている間は消せない）
aws acm delete-certificate --certificate-arn $CERT_ARN

# 5. Route 53 のレコード（A レコードと検証用 CNAME）→ コンソールで消すのが楽
#    NS と SOA は消せない（消す必要もない）

# 6. ホストゾーン（中のレコードが NS と SOA だけになってから）
aws route53 delete-hosted-zone --id $ZONE_ID

# 7. ドメインの自動更新をオフ（登録自体は期限まで残る）
aws route53domains disable-domain-auto-renew --domain-name $DOMAIN --region us-east-1
```

そのあと [Phase 1 の「全部消す」](DEPLOY-AWS.md#全部消す)へ。

---

## つまずきポイント

```bash
# まずこの 3 つを見る
aws elbv2 describe-target-health --target-group-arn $TG_ARN \
  --query 'TargetHealthDescriptions[].{State:TargetHealth.State,Reason:TargetHealth.Reason,Desc:TargetHealth.Description}'
aws logs tail /ecs/anime-manga-tracker --since 10m
aws ecs describe-services --cluster amt-cluster --services amt-service --query 'services[0].events[0:5]'
```

| 症状 | 原因 | 対処 |
| --- | --- | --- |
| ターゲットが `unhealthy`、Reason が `Target.Timeout` | タスク SG が ALB SG からの 8080 を許可していない | Step 8-1 |
| ターゲットが `unhealthy`、Reason が `Target.ResponseCodeMismatch` | `/healthz` が 503 を返している（= DB に繋がっていない） | RDS の状態と `amt-rds-sg` を確認。Phase 1 の手順書へ |
| ターゲットが `unused` のまま | ECS サービスに TG が紐付いていない | Step 7 の `update-service` が成功したか確認 |
| タスクが起動 → 停止を繰り返す | `--health-check-grace-period-seconds` を設定していない | Step 7。起動中のタスクが unhealthy 判定で殺されている |
| ブラウザで **503 Service Temporarily Unavailable** | healthy なターゲットが 0。デプロイ中か、タスクが落ちている | `describe-target-health` と ECS のログ |
| ブラウザで **504 Gateway Timeout** | タスクは登録されているが応答が返らない | タスク SG / コンテナのポート（8080）を確認 |
| ブラウザで **502 Bad Gateway** | ターゲットのポートが違う（8080 以外を指している） | TG のポート設定 |
| `ERR_CERT_COMMON_NAME_INVALID` | 証明書のドメインとアクセスしたホスト名が違う | `www` で来たなら証明書に `*.example.com` が要る（Step 2） |
| ブラウザで「安全ではありません」 | HTTP で開いている / リダイレクトが未設定 | Step 5-1 |
| ドメインが名前解決できない | ① A レコード未作成 ② 反映待ち ③ ドメイン登録が未完了 | `dig +short $DOMAIN` / Route 53 のホストゾーンを確認 |
| ACM が「検証保留中」のまま | 検証用 CNAME が入っていない | Step 2-1 の「Route 53 でレコードを作成」を押したか |
| ACM の証明書が ALB の選択肢に出ない | 証明書のリージョンが違う | ACM は `ap-northeast-1` で取り直す |
| ターゲットグループに ECS が登録できない | ターゲットタイプを `インスタンス` で作った | **TG を作り直す**（Fargate は IP のみ） |
| ALB が作れない（サブネットのエラー） | AZ が 1 つしか選ばれていない | パブリックサブネットを 2 つとも選ぶ |
| ログインできない / ログイン状態が保持されない | `COOKIE_SECURE` と古い Cookie の食い違い | ブラウザの Cookie を消す。シークレットウィンドウで確認 |
| 普通に使っているだけで 429 が出る / 別回線からでも 429 | `TRUST_PROXY` が `false` のまま。全員が ALB の IP で 1 つのバケットを共有している | Step 9 に戻り、タスク定義の `environment` に `TRUST_PROXY=true` があるか確認 |
| デプロイのたびに 5 分かかる | `deregistration_delay` が既定の 300 秒 | Step 4-1 |
| デプロイ中に 503 が出る | 仕様（単一タスク制約） | 上の「デプロイ」を参照。直すなら ElastiCache が先 |
| 15 日後に突然繋がらなくなった | 登録者メールの検証をしていない | Step 1-1。Route 53 → ドメインでステータスを確認 |

---

## 付録 A: サービスを作り直す場合

Step 7 の `update-service --load-balancers` が拒否された場合のみ。

```bash
aws ecs update-service --cluster amt-cluster --service amt-service --desired-count 0
aws ecs delete-service --cluster amt-cluster --service amt-service --force
aws ecs wait services-inactive --cluster amt-cluster --services amt-service

aws ecs create-service \
  --cluster amt-cluster \
  --service-name amt-service \
  --task-definition anime-manga-tracker \
  --desired-count 1 \
  --capacity-provider-strategy capacityProvider=FARGATE_SPOT,weight=1 \
  --network-configuration "awsvpcConfiguration={subnets=[${SUBNET_PUB1},${SUBNET_PUB2}],securityGroups=[${TASK_SG}],assignPublicIp=ENABLED}" \
  --deployment-configuration "minimumHealthyPercent=0,maximumPercent=100,deploymentCircuitBreaker={enable=true,rollback=true}" \
  --load-balancers targetGroupArn=${TG_ARN},containerName=app,containerPort=8080 \
  --health-check-grace-period-seconds 120 \
  --enable-execute-command

aws ecs wait services-stable --cluster amt-cluster --services amt-service
```

Phase 1 の `create-service` との差分は `--load-balancers` と
`--health-check-grace-period-seconds` の 2 つだけ。

---

## この後

Phase 1・2 を手で一周したので、次は **[Terraform 化](DEPLOY-AWS.md#この後-terraform-化)**。
`BACKLOG.md` に書いたとおり、**一度すべて削除して Terraform で作り直す**。

Phase 2 で増えたリソース（ACM・ALB・ターゲットグループ・リスナー 2 本・Route 53 レコード・
サインアップを閉じるリスナールール）は、Terraform だと `aws_lb` / `aws_lb_target_group` /
`aws_lb_listener` / `aws_acm_certificate` / `aws_route53_record` / `aws_lb_listener_rule` に
1 対 1 で対応する。手で作った順番がそのままリソースの依存関係になる。

CI/CD を繋ぐのもこのタイミング。GitHub Actions から **OIDC で AssumeRole** し、
`ecr push` → `register-task-definition` → `update-service` を回す。
`.github/workflows/ci.yml` は現在 build までで、デプロイは未接続。
