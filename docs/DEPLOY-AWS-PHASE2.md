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
| 月額 | $20 前後 | **$45〜50 前後** |

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
| 7 | タスク SG に ALB SG からの 8080 を許可 | ヘルスチェックの疎通 |
| 8 | ECS サービスに TG を紐付け | 動作確認 |
| 9 | タスク SG から自分の IP のルールを削除 | — |
| 10 | `COOKIE_SECURE=true` + `TRUST_PROXY=true` | — |
| 11 | 動作確認 | — |

**順番が大事。** ALB のヘルスチェックもタスク SG を通るので、**Step 7 を先にやらないと
Step 8 のヘルスチェックが一度も届かない**。120 秒の猶予が切れるとタスクが置換され、
数回の失敗でデプロイが FAILED になる。

逆に **Step 9（自分の IP のルール削除）は Step 8 の確認が済んでから**。先に塞ぐと、
ALB 経由が繋がらなかったときに確認する手段が無くなる。

### 所要時間とお金

- 所要時間: **2〜3 時間**（ドメイン登録の反映とACM の検証で待つ時間を含む）
- 追加月額:

| 項目 | 金額 | 備考 |
| --- | --- | --- |
| ALB | **月 $16.5 前後** | $0.0225/時 × 730。+ LCU 従量課金だがこの規模ではほぼゼロ |
| **パブリック IPv4（ALB ぶん）** | **月 $7.3** | $0.005/時 × 730 × 2。**ALB は AZ ごとに 1 つ IP を持つ** |
| Route 53 ホストゾーン | **月 $0.50** | ドメイン購入時に自動で 1 つ作られる |
| ドメイン登録 | **年 $14 前後**（.com） | TLD による。**返金されない** |
| ACM 証明書 | **無料** | ALB で使う限り |

Phase 1 の $20 と合わせて **月 $45〜50 前後**。

> **パブリック IPv4 は 2024 年から課金対象。** 見落としやすい。この構成では ALB に 2 つ、
> タスクに 1 つで合計 3 つ（月 $11 前後）。うちタスクの 1 つは Phase 1 から発生している。

> **ALB は「停止」できない。** RDS や ECS タスクと違い、使わない間だけ止める方法が無い。
> 放置すると**月 $24 前後**（ALB 本体 + パブリック IPv4 2 つ）が積み上がる。
> 長期間触らないなら ALB ごと削除すること（「運用」に手順）。

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

## Step 7: タスク SG に ALB SG からの 8080 を許可する

**ターゲットグループを紐付ける前にここを開ける。** ALB のヘルスチェックもタスク SG を通るので、
先に紐付けるとヘルスチェックが一度も届かない。

```bash
aws ec2 authorize-security-group-ingress \
  --group-id $TASK_SG --protocol tcp --port 8080 --source-group $ALB_SG
```

> **これで公開範囲は広がらない。** Step 8 までターゲットグループは空なので、ALB には
> 転送先が無い。外から実際に届くようになるのは Step 8 の瞬間で、それは順番を変えても同じ。
>
> **自分の IP のルールはまだ消さない。** ALB 経由が繋がらなかったときの確認手段として残す。
> 消すのは Step 9。

---

## Step 8: ECS サービスをターゲットグループに紐付ける

```bash
aws ecs update-service \
  --cluster amt-cluster \
  --service amt-service \
  --desired-count 1 \
  --load-balancers targetGroupArn=$TG_ARN,containerName=app,containerPort=8080 \
  --health-check-grace-period-seconds 120

aws ecs wait services-stable --cluster amt-cluster --services amt-service
```

- **`containerName` はタスク定義のコンテナ名** (`app`)。違うと紐付けに失敗する
- **`--desired-count 1` を明示する。** [Phase 1 の「費用を止める」](DEPLOY-AWS.md#費用を止めるしばらく触らないとき)で
  0 にしたまま来ると、タスクが無いまま `wait services-stable` が即座に成功し、
  「安定したのにブラウザは 503」という分かりにくい状態になる
- **`--health-check-grace-period-seconds 120` は必須級。**
  これが無いと、タスク起動直後（マイグレーション適用中でまだ listen していない時間）に
  ALB のヘルスチェックが落ちて ECS がタスクを殺す。猶予時間の間は失敗を無視する

> ローリング更新（ECS デプロイコントローラ）のサービスなら、ロードバランサー構成は
> **後から変更できる**。もしこのコマンドが拒否された場合は、サービスを作り直す
> （[付録 A](#付録-a-サービスを作り直す場合)）。

### 8-1. ターゲットが healthy になるのを確認する

```bash
aws elbv2 describe-target-health --target-group-arn $TG_ARN \
  --query 'TargetHealthDescriptions[].{IP:Target.Id,Port:Target.Port,State:TargetHealth.State,Reason:TargetHealth.Reason}' \
  --output table
```

`State` が `healthy` になるまで 1〜2 分。`unhealthy` のままなら「つまずきポイント」へ。

healthy になったら、**ブラウザで `https://<ドメイン>` を開いて表示されることを確認する。**
ここまでで ALB 経由の経路が通る。

> **ここで `unhealthy` が続く場合、放置しないこと。** 120 秒の猶予が切れるとタスクが
> 置換され、数回の失敗でデプロイが FAILED になる。`deploymentCircuitBreaker` が付いているので
> 無限ループにはならないが、**ロールバックしてもロードバランサー構成は外れない**ため、
> 原因を直すまで healthy なターゲットは 0 のままになる。まず Step 7 を実行したか確認する。

---

## Step 9: タスク SG から自分の IP のルールを消す

**Step 8-1 でブラウザ表示まで確認してから。** これで経路が ALB 経由だけになる。

いま入っているルールを見る:

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

## Step 10: COOKIE_SECURE と TRUST_PROXY を true にする

**2 つとも同じタイミングで切り替える。** どちらも「ALB が前に入った」ことで正解が変わる設定で、
Step 9 で SG を ALB からのみに閉じた今がその境目にあたる。

| 変数 | なぜ切り替えるか | 忘れると |
| --- | --- | --- |
| `COOKIE_SECURE` | HTTPS になったので Cookie に `Secure` を立てる | Cookie が平文 HTTP でも送られる状態のまま。HTTPS 化の効果が半減する |
| `TRUST_PROXY` | クライアント IP を `X-Forwarded-For` から取る | **全利用者が 1 つのレート制限バケットを共有し、数人が普通に使っただけで 429 になる** |

`TRUST_PROXY` のほうは**サイトが使えなくなる**ので、取りこぼすと痛い。理屈はこう:

- Step 9 でタスク SG を ALB SG からのみに閉じたため、`RemoteAddr` は**常に ALB** になった
- `TRUST_PROXY=false` のままだと、レート制限のキーが全員 ALB の IP になる
- 逆に Phase 1 で `true` にすると、クライアントが `X-Forwarded-For` を自分で書くだけで
  レート制限を回避できてしまう。**どちらの値も、前段の構成と対でしか正しくない**

詳細は `internal/config/config.go` の `TrustProxy` と `internal/handler/router.go` の
`IPExtractor` のコメントにある。

> **前提: 動いているイメージが `4e6f346` 以降であること。** `TRUST_PROXY` を読むコードと
> レート制限そのものは `4e6f346`（2026-10-13）で入った。Phase 1 のイメージ（`6c548cb`）は
> この変数を知らないので、**タスク定義に足しても黙って無視され、エラーも出ない**。
> 2026-10-22 に実際この状態で公開していた。先に Phase 1 の
> [「新しいバージョンをデプロイする」](DEPLOY-AWS.md#新しいバージョンをデプロイする)で
> `main` の最新を載せ、最後の確認でタグが一致してから 10-1 に進む。

### 10-1. タスク定義を書き換えて登録する

**`sed` で置換してはいけない。** Phase 1 のタスク定義に `TRUST_PROXY` は存在せず、
`sed` は置換対象が無くても終了コード 0 で成功するため、**黙って何も変えずに通る**。
`jq` で「既にあるものを除いてから足す」形にすれば、有無にかかわらず冪等に効く。

```bash
aws ecs describe-task-definition --task-definition anime-manga-tracker \
  --query 'taskDefinition' --output json \
  | jq '
      del(.taskDefinitionArn,.revision,.status,.requiresAttributes,
          .compatibilities,.registeredAt,.registeredBy,.deregisteredAt)
      | .containerDefinitions[0].environment |=
          ( map(select(.name != "COOKIE_SECURE" and .name != "TRUST_PROXY"))
            + [{name:"COOKIE_SECURE",value:"true"},{name:"TRUST_PROXY",value:"true"}] )
    ' > /tmp/amt-taskdef.json

# 2 つとも true で、重複していないことを確認してから登録する
jq '.containerDefinitions[0].environment' /tmp/amt-taskdef.json

aws ecs register-task-definition --cli-input-json file:///tmp/amt-taskdef.json

aws ecs update-service --cluster amt-cluster --service amt-service \
  --task-definition anime-manga-tracker
aws ecs wait services-stable --cluster amt-cluster --services amt-service
```

> `register-task-definition` が知らないフィールドで弾いたら、そのキーを `del(...)` に足す。
> API にフィールドが増えても同じ直し方で通る。

> **切り替え後、一度ログインし直すことになる。**
> `internal/handler/auth_handler.go` のコメントにあるとおり、ブラウザは `Secure` 属性が
> 食い違う Cookie を**別物として扱う**。Phase 1 で発行された `Secure` なしの Cookie が残っていると
> 上書きできず、挙動が不安定になる。ブラウザの Cookie を消すか、シークレットウィンドウで確認する。

---

## Step 11: 動作確認

### 11-1. リダイレクトと証明書

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

### 11-2. 旧経路が塞がったことの確認

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

### 11-3. ブラウザで

1. `https://<ドメイン>` を開く → **鍵マークが出る**
2. **Cookie を一度消してから**サインアップ / ログイン
3. 開発者ツール → Application → Cookies で、セッション Cookie に **`Secure` と `HttpOnly` が付いている**
4. ホーム画面に今季アニメ・話題の漫画が出る（外向き通信が生きている）
5. 検索 → ライブラリに追加（DB 書き込み）

### 11-4. レート制限が IP ごとに効いているか（TRUST_PROXY の検証）

**ここを飛ばさないこと。** `TRUST_PROXY` を取りこぼしていると、全利用者が ALB の IP で
1 つのバケットを共有する。自分 1 人で触っている間は気付けず、人が増えてから 429 で露見する。

> **2026-10-25 から `/api/home/*` を含む外部 API の `/api/*` はログイン必須（BACKLOG 4-a の 4）。**
> 未ログインの `curl` には `401` が返る。IP 単位の制限はログインの確認より先に効くので、この検証は
> `200` を `401` と読み替えればそのまま使える。**`401` は「制限に掛かっていない」の意味**で、設定ミスではない。

```bash
# API を 40 回連続で叩く（バースト 30 / 毎秒 2 本なので途中で 429 に入る）
for i in $(seq 1 40); do
  curl -s -o /dev/null -w '%{http_code} ' "https://${DOMAIN}/api/home/trending"
done; echo
# 401 が 30 強、そのあと 429 が混じる ← バケットが効いている
```

**判定はここから。2 つとも確認する。**

**(1) 別の IP なら別のバケットか。** 上で 429 を出したすぐあとに、**別の回線**
（スマホのモバイル通信、テザリングを切った状態）から 1 本だけ叩く。

```bash
curl -s -o /dev/null -w '%{http_code}\n' "https://${DOMAIN}/api/home/trending"
```

| 結果 | 意味 |
| --- | --- |
| `401` | **正しい。** IP ごとに別バケットになっている（2026-10-25 より前の版では `200`） |
| `429` | **設定ミス。** 全員が同じバケットを共有している |

**(2) XFF を偽装してもバケットが変わらないか。** `internal/handler/router.go` の
`IPExtractor` のコメントが「ALB を入れたら一度確認すること」と明記している項目。
429 が出ている状態のまま、ヘッダを足して 1 本叩く。

```bash
curl -s -o /dev/null -w '%{http_code}\n' \
  -H 'X-Forwarded-For: 1.2.3.4' "https://${DOMAIN}/api/home/trending"
```

| 結果 | 意味 |
| --- | --- |
| `429` | **正しい。** 偽装した値では別バケットにならない |
| `401` | **危険。** ALB の XFF 処理が既定の append になっていない。ヘッダを書き換えるだけでレート制限を回避できる状態（2026-10-25 より前の版では `200`） |

> `echo.ExtractIPFromXFFHeader()` は XFF を**右から左へ**辿り、最初に「信頼しない」IP を返す。
> ALB の `10.0.x.x` は既定の信頼範囲（RFC1918）なので飛ばされ、次に当たるのが ALB が追記した
> 本物の接続元になる。クライアントが自分で書いた値は必ずそれより左にあるので届かない。

`429` が返らなかった場合、あるいは (1) で `429` が返った場合は、タスク定義を確認する。

```bash
aws ecs describe-task-definition --task-definition anime-manga-tracker \
  --query 'taskDefinition.containerDefinitions[0].environment' --output table
```

### 11-5. ターゲットとログ

```bash
aws elbv2 describe-target-health --target-group-arn $TG_ARN \
  --query 'TargetHealthDescriptions[].TargetHealth.State' --output text
# healthy

aws logs tail /ecs/anime-manga-tracker --since 10m
```

> ヘルスチェックはアプリのログには出ない（このアプリはリクエストログのミドルウェアを
> 入れていないため）。30 秒ごとのアクセスでログが溢れる心配はない。

> **`DATABASE_URL` がログに出ないかの確認（BACKLOG 4-d）は、ここではやらない。**
> コードを読んで決着済み。`pgxpool.New` のパースエラーは pgx の `ParseConfigError` で、
> `Error()` が `redactPW()` を通すため**パスワードは `xxxxx` に置き換わる**
> （ホスト名・ユーザー名・DB 名は残る）。`pool.Ping` 側の失敗は接続文字列を持たない。
> わざと壊す検証は SSM の値を書き換える事故リスクに見合わない。

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

> **これでは ALB の月 $24 前後は止まらない。** ALB に停止の概念は無く、存在するだけで
> 課金される（本体 $16.5 + パブリック IPv4 2 つ $7.3）。数週間以上触らないなら ALB ごと削除する:
>
> ```bash
> aws elbv2 delete-load-balancer --load-balancer-arn $ALB_ARN
> # Route 53 の A レコードも参照先が消えるので削除する（コンソールから）
> ```
>
> ターゲットグループ・証明書・ホストゾーンは残しておいてよい（TG と ACM は無料、
> ホストゾーンは月 $0.50）。ALB を作り直したら Step 5-1・6・8 をやり直す
> （Step 7 のタスク SG のルールは ALB SG を参照しているので、SG を消していなければ残る）。

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
# 1. ECS サービスを先に消す（ターゲットグループを参照しているため）
aws ecs update-service --cluster amt-cluster --service amt-service --desired-count 0
aws ecs delete-service --cluster amt-cluster --service amt-service --force
aws ecs wait services-inactive --cluster amt-cluster --services amt-service

# 2. ALB（リスナーも一緒に消える）
aws elbv2 delete-load-balancer --load-balancer-arn $ALB_ARN
sleep 30

# 3. ターゲットグループ（ALB を消してから）
aws elbv2 delete-target-group --target-group-arn $TG_ARN

# 4. ALB の SG（ALB が完全に消えてから。ENI が残っていると失敗する）
aws ec2 delete-security-group --group-id $ALB_SG

# 5. ACM 証明書（ALB との関連付けが外れてから）
aws acm delete-certificate --certificate-arn $CERT_ARN

# 6. Route 53 のレコード（A レコードと検証用 CNAME）→ コンソールで消すのが楽
#    NS と SOA は消せない（消す必要もない）

# 7. ホストゾーン（中のレコードが NS と SOA だけになってから）
aws route53 delete-hosted-zone --id $ZONE_ID

# 8. ドメインの自動更新をオフ（登録自体は期限まで残る）
aws route53domains disable-domain-auto-renew --domain-name $DOMAIN --region us-east-1
```

> **5 が `ResourceInUse` で失敗することがある。** ALB を消しても ACM 側の関連付けが
> 消えるまでに時間差がある。数分待って再実行する。急がないなら 6 以降を先に進めてよい
> （証明書は無料なので残っていても費用はかからない）。

> **7 でホストゾーンを消すと、ドメイン登録の NS 委任が宙に浮く。** ドメイン自体は
> 期限まで残るので、あとで作り直すときは**新しいホストゾーンの NS をドメイン登録側に
> 張り直す**必要がある。ドメインを使い続けるつもりならホストゾーンは消さないほうが楽
> （月 $0.50）。

そのあと [Phase 1 の「全部消す」](DEPLOY-AWS.md#全部消す)へ。
**ECS サービスは上の 1 で消してあるので、そこは飛ばす。**

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
| ターゲットが `unhealthy`、Reason が `Target.Timeout` | タスク SG が ALB SG からの 8080 を許可していない | Step 7。順番を飛ばすとここに来る |
| ターゲットが `unhealthy`、Reason が `Target.ResponseCodeMismatch` | `/healthz` が 503 を返している（= DB に繋がっていない） | RDS の状態と `amt-rds-sg` を確認。Phase 1 の手順書へ |
| ターゲットが `unused` のまま | ECS サービスに TG が紐付いていない | Step 8 の `update-service` が成功したか確認 |
| タスクが起動 → 停止を繰り返す | `--health-check-grace-period-seconds` を設定していない | Step 8。起動中のタスクが unhealthy 判定で殺されている |
| ブラウザで **503 Service Temporarily Unavailable** | healthy なターゲットが 0。デプロイ中か、タスクが落ちている | `describe-target-health` と ECS のログ |
| ブラウザで **504 Gateway Timeout** | ALB からタスクに届いていない、または応答が返らない | タスク SG（Step 7）と TG のポート設定（8080）を確認 |
| ブラウザで **502 Bad Gateway** | タスクには届いたが、不正な応答を返したか接続を切られた | ECS のログを見る。アプリが落ちている可能性 |
| `ERR_CERT_COMMON_NAME_INVALID` | 証明書のドメインとアクセスしたホスト名が違う | `www` で来たなら証明書に `*.example.com` が要る（Step 2） |
| ブラウザで「安全ではありません」 | HTTP で開いている / リダイレクトが未設定 | Step 5-1 |
| ドメインが名前解決できない | ① A レコード未作成 ② 反映待ち ③ ドメイン登録が未完了 | `dig +short $DOMAIN` / Route 53 のホストゾーンを確認 |
| ACM が「検証保留中」のまま | 検証用 CNAME が入っていない | Step 2-1 の「Route 53 でレコードを作成」を押したか |
| ACM の証明書が ALB の選択肢に出ない | 証明書のリージョンが違う | ACM は `ap-northeast-1` で取り直す |
| `wait services-stable` が `Max attempts exceeded` で終わる | ヘルスチェックが通らずデプロイが収束していない | 上 3 つのコマンドで原因を見る。Step 7 を飛ばしていないか |
| ターゲットグループに ECS が登録できない | ターゲットタイプを `インスタンス` で作った | **TG を作り直す**（Fargate は IP のみ） |
| ALB が作れない（サブネットのエラー） | AZ が 1 つしか選ばれていない | パブリックサブネットを 2 つとも選ぶ |
| ログインできない / ログイン状態が保持されない | `COOKIE_SECURE` と古い Cookie の食い違い | ブラウザの Cookie を消す。シークレットウィンドウで確認 |
| **RDS を止めている間、タスクが起動と停止を繰り返す** | `/healthz` が DB の `Ping` まで見て 503 を返し、ALB が unhealthy と判定して ECS がタスクを置換する。**Phase 2 で新しく出る挙動**（Phase 1 ではタスクは落ちずに 503 を返すだけだった） | RDS を起動する。止めておくなら ECS も `--desired-count 0` にしておく（「費用を止める」の順番どおりにやれば踏まない） |
| 普通に使っているだけで 429 が出る / 別回線からでも 429 | `TRUST_PROXY` が `false` のまま。全員が ALB の IP で 1 つのバケットを共有している | Step 10 に戻り、タスク定義の `environment` に `TRUST_PROXY=true` があるか確認 |
| XFF を偽装すると 429 を回避できる | ALB の XFF 処理が既定の append になっていない | Step 11-4 の (2)。ALB の属性 `routing.http.xff_header_processing.mode` を確認 |
| デプロイのたびに 5 分かかる | `deregistration_delay` が既定の 300 秒 | Step 4-1 |
| デプロイ中に 503 が出る | 仕様（単一タスク制約） | 上の「デプロイ」を参照。直すなら ElastiCache が先 |
| 15 日後に突然繋がらなくなった | 登録者メールの検証をしていない | Step 1-1。Route 53 → ドメインでステータスを確認 |

---

## 付録 A: サービスを作り直す場合

Step 8 の `update-service --load-balancers` が拒否された場合のみ。

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

## 2026-10-25 の機能を本番に出す

Phase 2 の公開構成のあとに入った機能（AI 翻訳・自然文検索・AI おすすめ・「新着」タブの新刊と新連載・既刊の巻数）と、
API のログイン必須化（BACKLOG 4-a の 4）をまとめて出す手順。**タスク定義の更新を 1 回にまとめる**ので、
落ちるのも 1 回（30〜60 秒）で済む。

**変わらないもの:** インフラ（VPC・SG・ALB・RDS）、IAM、ヘルスチェック（`/healthz` は今もログイン不要）。
外向きの通信は、AniList と同じくタスクのパブリック IP から Anthropic（`api.anthropic.com`）と楽天
（`openapi.rakuten.co.jp`）へ出る。NAT もエンドポイントも足さない。

### 0. AWS の外でやること（先に済ませる）

1. **Anthropic Console で月の利用上限（Spend limit）を設定する。** アプリの日次上限（200 万トークン）は
   プロセス内のメモリにあり、デプロイのたびに 0 に戻る（[Phase 1 の手順書](DEPLOY-AWS.md)の「AI（Claude）の API キーを入れる」）
2. **楽天ウェブサービスのアプリの「許可されたウェブサイト」に `${DOMAIN}` を入れる。** アプリの種類は
   「ウェブアプリケーション」（[楽天ブックスのキーを入れる](DEPLOY-AWS.md#楽天ブックスのキーを入れる一度だけ)）
3. **楽天アフィリエイト ID を取る。** 規約（第 3 条）が商品ページへのリンクをアフィリエイト経由で作るよう定めているため
   （BACKLOG 11-b「規約で確かめたこと」）。取れていなければ 4 の `RAKUTEN_AFFILIATE_ID` を空にして進めてもアプリは動くが、
   公開する前に入れること

### 1. キーを SSM に入れる

```bash
# 履歴にキーを残さないよう、ファイル経由で入れる。どれも SecureString
aws ssm put-parameter --name /amt/prod/ANTHROPIC_API_KEY --type SecureString \
  --value "$(cat /tmp/anthropic-key.txt)"
aws ssm put-parameter --name /amt/prod/RAKUTEN_APPLICATION_ID --type SecureString \
  --value "$(cat /tmp/rakuten-app-id.txt)"
aws ssm put-parameter --name /amt/prod/RAKUTEN_ACCESS_KEY --type SecureString \
  --value "$(cat /tmp/rakuten-access-key.txt)"
rm /tmp/anthropic-key.txt /tmp/rakuten-app-id.txt /tmp/rakuten-access-key.txt
```

すでに入れたものは飛ばしてよい（入れ直すなら `--overwrite`）。IAM の変更は要らない
（Phase 1 の Step 6-1 のインラインポリシーが `parameter/amt/prod/*` を許可している）。

### 2. イメージを作って push する

```bash
git checkout main && git pull
export TAG=$(git rev-parse --short HEAD)
aws ecr get-login-password --region ${AWS_REGION} \
  | docker login --username AWS --password-stdin ${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com
docker build --platform linux/amd64 -t ${ECR_REPO}:${TAG} .
docker push ${ECR_REPO}:${TAG}
```

### 3. タスク定義を 1 回で書き換えて反映する

イメージ・`secrets` 3 つ・`environment` 2 つをまとめて差し替える。`jq` で同名を除いてから足すので、何度流しても重複しない。

```bash
export RAKUTEN_AFFILIATE_ID=   # 3 で取った楽天アフィリエイト ID（まだなら空のまま）

aws ecs describe-task-definition --task-definition anime-manga-tracker \
  --query 'taskDefinition' --output json \
  | jq --arg img "${ECR_REPO}:${TAG}" \
       --arg prefix "arn:aws:ssm:${AWS_REGION}:${ACCOUNT_ID}:parameter/amt/prod/" \
       --arg referer "https://${DOMAIN}/" \
       --arg affiliate "${RAKUTEN_AFFILIATE_ID}" '
      del(.taskDefinitionArn,.revision,.status,.requiresAttributes,
          .compatibilities,.registeredAt,.registeredBy,.deregisteredAt)
      | .containerDefinitions[0].image = $img
      | .containerDefinitions[0].secrets |=
          ( map(select(.name as $n | ["ANTHROPIC_API_KEY","RAKUTEN_APPLICATION_ID","RAKUTEN_ACCESS_KEY"] | index($n) | not))
            + [ {name:"ANTHROPIC_API_KEY",      valueFrom:($prefix + "ANTHROPIC_API_KEY")},
                {name:"RAKUTEN_APPLICATION_ID", valueFrom:($prefix + "RAKUTEN_APPLICATION_ID")},
                {name:"RAKUTEN_ACCESS_KEY",     valueFrom:($prefix + "RAKUTEN_ACCESS_KEY")} ] )
      | .containerDefinitions[0].environment |=
          ( map(select(.name != "RAKUTEN_REFERER" and .name != "RAKUTEN_AFFILIATE_ID"))
            + [ {name:"RAKUTEN_REFERER", value:$referer} ]
            + (if $affiliate == "" then [] else [ {name:"RAKUTEN_AFFILIATE_ID", value:$affiliate} ] end) )
    ' > /tmp/amt-taskdef.json

# 登録する前に目で確かめる。COOKIE_SECURE / TRUST_PROXY（Step 10）が残っていること
jq '.containerDefinitions[0] | {image, environment, secrets}' /tmp/amt-taskdef.json
aws ecs register-task-definition --cli-input-json file:///tmp/amt-taskdef.json

aws ecs update-service --cluster amt-cluster --service amt-service \
  --task-definition anime-manga-tracker
aws ecs wait services-stable --cluster amt-cluster --services amt-service

# 動いているイメージのタグが ${TAG} と一致することを確認する（必須。Phase 1 の手順書の「最後の確認を省かない」）
aws ecs describe-task-definition --task-definition \
  $(aws ecs describe-services --cluster amt-cluster --services amt-service \
      --query 'services[0].taskDefinition' --output text) \
  --query 'taskDefinition.containerDefinitions[0].image' --output text
```

- `RAKUTEN_REFERER` は `${DOMAIN}` から作る。楽天の「許可されたウェブサイト」と同じドメインにすること（違うと楽天が 403 を返す）
- 日次上限を変えたいときは、同じ要領で `environment` に `CLAUDE_DAILY_TOKEN_LIMIT` を足す

### 4. 確かめる

```bash
# 起動ログ。new-releases-warm は 1 分半ほどかかるので、起動から 2 分待ってから見る
aws logs tail /ecs/anime-manga-tracker --since 5m \
  | grep -E 'migrate|ai: |rakuten|new-releases-warm|home-cache-warm'
# migrate: applied 0005_translations.up.sql ← 2026-10-22 より前のイメージからなら、このとき自動で適用される
#                                        （すでに適用済みなら "already up to date (5 migrations)"）
# ai: enabled (daily limit 2000000 tokens)
# rakuten: enabled                      ← アフィリエイト ID が無いと "(RAKUTEN_AFFILIATE_ID is not set: ...)" が付く
# scheduler: home-cache-warm done in ...
# scheduler: new-releases-warm done in 1m...

# API は未ログインなら 401（BACKLOG 4-a の 4）。ヘルスチェックは今も 200
curl -s -o /dev/null -w 'healthz %{http_code}\n'  "https://${DOMAIN}/healthz"
curl -s -o /dev/null -w 'trending %{http_code}\n' "https://${DOMAIN}/api/home/trending"
# healthz 200
# trending 401
```

ブラウザで（ログインしてから）:

1. 作品モーダルのあらすじが日本語になる（AI 翻訳）
2. 検索の「内容で探す」で「異世界で料理する漫画」が引ける。「薫子っていうキャラが出るアニメ」のようにキャラクター名でも引ける
3. ホームに「AI のおすすめ」の棚が出る（記録が 3 件以上あるとき。初回は数秒〜十数秒かかる）
4. 「新着」→「新刊」に今週のコミックが出る。日付とジャンルで絞れ、表紙を押すとダイアログが出る。下に
   「Supported by Rakuten Developers」が出る（楽天の規約で必須）
5. 「新着」→「新連載」に最近始まった連載が出る
6. 連載中の漫画のモーダルで「連載中（既刊 N 巻）」か「単行本は見つかりませんでした」が出る

うまくいかないとき:

| 症状 | 原因 |
| --- | --- |
| `ai: disabled` | `ANTHROPIC_API_KEY` が `secrets` に入っていない |
| `rakuten: disabled (set both ...)` | 楽天のキーが片方しか入っていない |
| `new-releases-warm failed ... 403 ... REQUEST_CONTEXT_BODY_HTTP_REFERRER_MISSING` | `RAKUTEN_REFERER` のドメインが楽天の「許可されたウェブサイト」に無い |
| `new-releases-warm failed ... 429` | 楽天のレート制限。しばらく待つと、次の取り直し（3 時間ごと）か新刊の画面を開いたときに取り直す |
| 新刊が「取得しています」のまま | 起動直後で `new-releases-warm` が終わっていない（1 分半ほど）。終わっても続くならログで `failed` を探す |
| AI の機能だけ従来の動き（キーワード検索・ジャンルのおすすめ）になる | 日次上限に達したか、Anthropic 側の失敗。ログで `nlsearch: ai failed` / `recommend: ai recommendations failed` を探し、理由に `daily token budget exceeded` があれば日次上限 |

**戻すとき:** 1 つ前のリビジョンを指定して `update-service` する（`--task-definition anime-manga-tracker:<前の番号>`）。
マイグレーション 0005（`translations`）は足すだけなので、古いイメージに戻しても動く。

**確かめていないこと:** メモリ（512MB）に収まるか。AI の条件・おすすめ・既刊・新刊のキャッシュはどれも件数に上限があり、
ローカルでは問題になっていないが、本番では測っていない。しばらくは CloudWatch の `MemoryUtilization` を見る。

## この後

Phase 1・2 を手で一周したので、次は **[Terraform 化](DEPLOY-AWS.md#この後-terraform-化)**。
`BACKLOG.md` に書いたとおり、**一度すべて削除して Terraform で作り直す**。

Phase 2 で増えたリソース（ACM・ALB・ターゲットグループ・リスナー 2 本・Route 53 レコード）は、
Terraform だと `aws_lb` / `aws_lb_target_group` / `aws_lb_listener` / `aws_acm_certificate` /
`aws_route53_record` に 1 対 1 で対応する。手で作った順番がそのままリソースの依存関係になる。

CI/CD を繋ぐのもこのタイミング。GitHub Actions から **OIDC で AssumeRole** し、
`ecr push` → `register-task-definition` → `update-service` を回す。
`.github/workflows/ci.yml` は現在 build までで、デプロイは未接続。

### HSTS を入れるか決める

`internal/handler/router.go` の `middleware.Secure()` は既定設定のままで、**HSTS を意図的に
出していない**。理由はコメントにあるとおり「`max-age` はブラウザに残り続けて取り消しが効かない」
ため。HTTPS での運用が固まったこの段階が、入れるかを判断するタイミングになる。

短い `max-age`（数分）から始めて、問題がなければ伸ばす。**`includeSubDomains` と `preload` は
後戻りできないので、入れるなら最後**。ドメインを手放す予定があるなら、そもそも入れないほうがよい。
