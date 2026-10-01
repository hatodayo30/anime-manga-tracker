# バックログ（残タスクと着手順）

残っている作業と、着手する順番・依存関係をまとめた文書。**ここが残タスクの唯一の正**とし、
更新しながら使う。個々のタスクの詳細な仕様や調査経緯は `prompt*.md` を参照すること
（promptN.md は書かれた時点の記録なので、着手順については本ファイルを優先する）。

最終更新: 2026-10-01

---

## 実装済み

| 出典 | 内容 |
| --- | --- |
| prompt.md | 初期構築、検索の人気順ソート、アダルト除外、タイトルの native→romaji→english フォールバック |
| prompt2.md | UI リデザイン、作品モーダル、進捗 +/- UI、おすすめページ |
| prompt3.md | 評価（★5段階）・メモ（`migrations/0003`） |
| prompt5.md | ライブラリの並び替え、漫画の巻数トラッキング（`migrations/0004`） |
| prompt6.md | シーズン画面のキャッシュ、single-flight、AniList のトークンバケット化 |

---

## 着手順

### 1. stale-while-revalidate（prompt7.md 着手 1）

TTL 切れでも古い値を即返し、更新はバックグラウンドで走らせる。

- **なぜ最初か**: 遅さの主因（キャッシュミス時に Jikan 10 件で 3 秒以上待つ）を直接潰せて、
  変更が `internal/usecase/cache` の中だけで閉じる。追加インフラもデプロイ先の決定も不要。
- **依存**: なし。今すぐ着手できる。
- **先に決めること**: 古い値を返し続けてよい上限（例: TTL の 10 倍）。際限なく返すと、
  上流が落ちている間ずっと腐ったデータを配ることになる。

### 2. バックグラウンド実行基盤

`cmd/server/main.go` は Echo 単一プロセスで、スケジューラもジョブランナーも持っていない。
下の 2-a と 2-b は**どちらも同じ基盤を必要とする**ので、先にこれを入れて両方を乗せる。

**デプロイ先は ECS Fargate に決定済み**（下記 3）。`desiredCount: 1` の常時起動タスクなので、
ticker で回す素直な実装でよい。ただし**デプロイ戦略を単一タスク保証に設定すること**が前提で、
怠るとローリングデプロイ中にジョブが二重実行される（3 を参照）。

- **2-a. キャッシュのウォームアップ + 定期リフレッシュ**（prompt7.md 着手 2）
  起動時に温め、その後も定期的に更新して、ユーザーが冷えたキャッシュを踏まないようにする。
- **2-b. `next_airing_at` の定期更新ジョブ**（prompt4.md 候補 2-a）
  現状、更新経路が存在しない。`internal/infrastructure/postgres/record_repository.go` の
  Upsert がクライアントから渡された値を書くだけなので、ライブラリ追加とステータス変更時しか
  更新されず、既存レコードの次回放送日時は腐ったまま。
  これを直すと、ライブラリの「次話」表示と既定ソート（`next_airing_at ASC`）が正しくなる。
  通知機能を作らなくても単体で元が取れる。

**注意**: 2-a・2-b とも AniList の共有スロットルをユーザーリクエストと奪い合う。
平均レート上限（30req/分）は変わらないので、低頻度・バッチにまとめる設計にすること。

### 3. AWS へのデプロイ（ECS Fargate）

**決定済み（2026-10-01）**: ECS Fargate を使う。AWS の学習が目的で、実務で最も見る構成のため。
コストは構成で抑え、ALB は Phase 2 まで入れない。

#### なぜ Fargate か（トレードオフの記録）

Lightsail のほうが安く、ticker も素直に動き、`docker-compose.yml` がそのまま載る。それでも
Fargate を選ぶのは**学習が目的だから**であり、このアプリの要件から導かれた選択ではない。
以下を承知の上で進める。

- **水平スケールは封印する**（`desiredCount: 1` 固定、Service Auto Scaling は設定しない）。
  prompt6.md のキャッシュ・single-flight・AniList のトークンバケットはすべてプロセス内にあり、
  2 タスクになると実効レートが倍になって 429 を踏む。ElastiCache に出すまで解禁しない。
- **コストの主因は Fargate 本体ではなく周辺リソース**。Spot 最小構成の月$3 前後に対し、
  ALB は月$18 前後、NAT Gateway は月$33 前後。NAT は作らない。

#### コスト方針

| 項目 | 方針 |
| --- | --- |
| NAT Gateway | **作らない**。タスクをパブリックサブネットに置き `assignPublicIp: ENABLED`、SG で閉じる |
| VPC エンドポイント | 作らない。Interface 型は 1 つ月$7〜で、NAT 回避の目的にはこの規模では割高 |
| Fargate | **Spot**（0.25 vCPU / 0.5 GB）。中断は許容する |
| ALB | Phase 2 まで入れない |
| DB | RDS db.t4g.micro（単一 AZ）。EFS + Postgres 同居は Postgres が NFS 非推奨のため採らない |
| シークレット | SSM Parameter Store の SecureString（標準パラメータは無料。Secrets Manager は $0.40/件・月） |
| CloudWatch Logs | 保持期間を明示設定（既定は無期限保持） |
| ECR | ライフサイクルポリシーで最新 10 イメージのみ保持 |

概算は Phase 1 で月$20 前後、Phase 2 で月$38 前後。**正確な額は AWS Pricing Calculator で確認すること。**

#### Phase 0: コード側の前提（デプロイ作業より先・ローカルで完結）— **完了（2026-10-01）**

- ~~**`/healthz` エンドポイントの追加**~~ — 実装済み（`internal/handler/health_handler.go`）。
  DB の `Ping` までに留め、AniList / Jikan は叩かない。タイムアウトは 2 秒
  （ECS のコンテナヘルスチェックの既定 5 秒より短くして、こちらから 503 を返せるようにした）。
- ~~**マイグレーションランナーの実装**~~ — 実装済み。`migrations/embed.go` が `*.sql` を
  `embed.FS` に焼き込み、`internal/infrastructure/postgres/migrate.go` が起動時に未適用分だけを
  バージョン昇順で適用する。適用済みは `applied_migrations` テーブルに記録。
  - `golang-migrate` は**入れなかった**。あちらの DB ドライバは `database/sql` 前提で、
    既存の `pgxpool` とは別に接続を張ることになる。対象 4 本・単一タスクに対して
    依存を増やす釣り合いが取れないと判断した。乗り換え余地を残すため、記録テーブル名は
    `schema_migrations` ではなく `applied_migrations` にしてある。
  - 単一タスク固定なので本来不要だが、`pg_advisory_lock` は掛けてある（デプロイ戦略の
    設定ミスや手元からの直接実行と重なったときの保険。取得コストはほぼゼロ）。
  - **既存のローカル DB への配慮**: `docker-entrypoint-initdb.d` で初期化済みの `pgdata`
    ボリュームには 0001〜0004 が適用されているが記録テーブルが無い。そのまま流すと 0001 の
    `CREATE TABLE` が衝突し、0004 の `UPDATE` が漫画の進捗を消す。`records` テーブルの存在を
    検出したら `baselineVersion`（= 4）までを適用済みとして記録してから差分を流す。
  - `docker-compose.yml` の initdb マウントは削除した。ローカルと RDS で適用経路を分けない。
  - `Dockerfile` に `COPY migrations/ migrations/` を追加（embed はビルド時に SQL を要求する）。
- ~~**セッション Cookie の `Secure` 属性を環境変数で切り替え可能にする**~~ — 実装済み。
  `COOKIE_SECURE`（既定 `false`）。真偽値として読めない値は既定値に倒さず起動時エラーにする
  （`ture` のような綴り間違いで `Secure` が黙って外れるのを防ぐ）。
  Phase 2 の HTTPS 化で `COOKIE_SECURE=true` にすること。

#### Phase 1: 最小構成で動かす（ALB なし）

**これは学習用の足場であって本番構成ではない。** 平文 HTTP なので、SG のインバウンドは
自分の IP のみに制限し、常用しないこと。

構成要素:

- VPC（パブリックサブネット×2・プライベートサブネット×2、NAT なし）
- ECR リポジトリ（ライフサイクルポリシー付き）
- RDS db.t4g.micro（プライベートサブネット、パブリックアクセス無効、SG はタスク SG からの 5432 のみ）
- ECS クラスタ + タスク定義（Fargate Spot、awslogs、`secrets` で SSM から `DATABASE_URL` を注入）
- ECS サービス（`desiredCount: 1`、`assignPublicIp: ENABLED`、`enableExecuteCommand: true`）
- IAM はタスク実行ロール（ECR pull・ログ書き込み）とタスクロール（アプリ自身の AWS 呼び出し、現状なし）を分ける

**必ず設定すること**: デプロイ戦略を `minimumHealthyPercent: 0` / `maximumPercent: 100` にする。
既定（100 / 200）だとローリングデプロイ中に 2 タスクが並走し、AniList のレートを二重に消費する。
数十秒のダウンタイムと引き換えに単一インスタンスを保証する。

DB の調査やアドホックな操作は **ECS Exec**（`aws ecs execute-command`）で実行中タスクに入って行う。
踏み台ホストは立てない。

アクセスは `http://<タスクのパブリック IP>:8080`。**Fargate では Elastic IP を固定できない**ため、
IP は再起動・Spot 中断・デプロイのたびに変わる。

#### Phase 2: 公開構成（ALB）

ALB・ACM 証明書・Route53 を追加し、タスク SG のインバウンドを ALB SG からのみに変更する。
HTTP → HTTPS リダイレクトを設定し、Cookie の `Secure` を有効化する。ここで初めて常用できる。

#### IaC の進め方

**最初の 1 周はマネジメントコンソールで手作業で作る。** クラスタ・サービス・タスク定義・
ターゲットグループの関係を知らないまま IaC を書くと、エラーの意味が読めない。
一度動かして理解したら**削除し、Terraform で作り直す**。

Terraform を推奨（実務で最も見る）。CDK は L2 コンストラクトが裏で大量のリソースを自動生成する
ため、学習目的では何が作られたのか見えにくい。

CI/CD は GitHub Actions から **OIDC で AssumeRole**（アクセスキーを Secrets に置かない）。
`.github/workflows/ci.yml` は現在 build までで、デプロイは未接続。

#### 着手時期

**1・2 の実装とは独立**しており、並行して進めてよい。1 の効果はインフラの選択と無関係だから。
むしろ早めにデプロイすると、現在「3〜5 秒」としている遅さがスロットル定数からの計算値である
ところを実測に置き換えられ、1 がどれだけ効くかが数字で分かる。

**Spot 中断と再デプロイのたびにプロセス内キャッシュが空になる**ので、Fargate を選んだことで
1（stale-while-revalidate）と 2-a（ウォームアップ）の価値はむしろ上がった。

学習が一段落して放置する場合、**費用の主因は RDS** になる。`terraform destroy` できる状態に
しておくこと（IaC 化のもう一つの実利）。

詳細は prompt7.md を参照。

### 4. 統計ダッシュボード（prompt4.md 候補 3）

**着手前に判断が必要**: 月別視聴本数を出すなら `completed_at` 相当のカラムが要る
（`created_at` は追加日、`updated_at` は評価の編集でも動くため代用できない）。
足すか、月別を諦めるかを先に決めること。

ステータス別内訳とジャンル分布は既に `LibraryPage.tsx` にあるので、専用ページの新規価値は
「もっと詳しく出る」程度。スコープを絞ること。

### 5. タイトルの JA/EN 切替ボタン（prompt.md）

サーバー側のフォールバック（`internal/anilist/queries.go`、日本作品は native 優先、
韓国・中国作品は english 優先）は実装済みだが、**ユーザーが切り替える UI がない**。
表示言語をサーバーが決め打ちしている状態。

面が小さく他に依存しないので、いつ入れてもよい。

### 6. Web Push 通知（prompt4.md 候補 2-b）

**最もコストが大きいので最後。** 2-b（`next_airing_at` の更新ジョブ）が終わっていることが前提。

- PWA 基盤がゼロ（`frontend/public/` は `favicon.svg` のみ。manifest も Service Worker もない）
- iOS Safari の Web Push は「ホーム画面に追加された PWA」でないと動かない。
  モバイル前提の UI なので、ここが効果を大きく削る

---

## 失効した仕様

- **prompt2.md のホーム画面仕様**（統計サマリーバッジ、右カラムの「積み作品＋今季人気 TOP5」）
  未実装だが、`5cb6457` のリデザインでホームは「今週放送の人気アニメ TOP10 + 曜日別スケジュール
  + 話題の漫画 TOP10」に置き換わった。未実装というより失効扱いとする。
