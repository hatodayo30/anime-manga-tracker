# anime-manga-tracker

観ているアニメ・読んでいる漫画の進捗を記録し、今季の新作や人気作を探せる Web アプリです。
作品情報は [AniList GraphQL API](https://anilist.co) から取得し、MAL のランキングや掲載誌は
[Jikan API](https://jikan.moe) で補完しています。

Go (Echo) のシングルバイナリが API と React 製フロントエンドの静的ファイルを両方配信します。

## 主な機能

- **ホーム** — 今週放送の人気アニメ TOP10、曜日別の放送スケジュール、話題の漫画 TOP10（ログイン不要）
- **シーズン一覧** — 年（2020 年以降）とシーズンを選んで放送作品をブラウズ（40 件を 20 件 × 2 ページ）
- **検索** — AniList からアニメ / 漫画を検索してライブラリに追加
- **マイライブラリ** — 「見たい / 見てる / 見た」のステータス管理、進捗（アニメ=話数・漫画=巻数）、
  5 段階評価、メモ。並び替えは 次回放送が近い順 / タイトル / 評価 / 更新順 / 追加順
- **おすすめ** — ライブラリのジャンル傾向から作品を提案
- **作品モーダル** — あらすじ（MyMemory API で日本語へ機械翻訳）、関連作品（続編・前日譚など）
- **認証** — メール + パスワードのサインアップ / ログイン。セッションは HttpOnly Cookie（有効期限 30 日）

## 技術スタック

| レイヤ | 使用技術 |
| --- | --- |
| バックエンド | Go 1.25 / [Echo v4](https://echo.labstack.com) / [pgx v5](https://github.com/jackc/pgx) |
| フロントエンド | React 19 / TypeScript / Vite / React Router v7 |
| DB | PostgreSQL 16 |
| 外部 API | AniList (GraphQL) / Jikan (MyAnimeList) / MyMemory Translation |
| CI | GitHub Actions（gofmt・go vet・go build / oxlint・tsc・vite build） |

いずれの外部 API も API キー不要です。

## ディレクトリ構成

```
cmd/server/          エントリポイント（DI と HTTP サーバーの起動）
internal/
  domain/            ドメインモデル（Record, User, Status, MediaType, SortKey）
  usecase/           ユースケース層（auth / record / search / home / translate / cache）
  handler/           HTTP ハンドラとルーティング、静的ファイル配信
  middleware/        セッション Cookie による認証ミドルウェア
  infrastructure/postgres/  リポジトリ実装（pgx）
  anilist/ jikan/ translate/  外部 API クライアント
migrations/          SQL マイグレーション（up / down）
frontend/            React + Vite のフロントエンド
```

usecase 層はリポジトリ / ゲートウェイのインターフェースを自分で定義し、infrastructure と
外部 API クライアントがそれを実装します（依存性逆転）。外部 API の応答は
`internal/usecase/cache` の TTL キャッシュ（single-flight 付き）で短時間保持します。

## セットアップ

### Docker Compose で動かす

```sh
docker compose up --build
```

http://localhost:8090 で起動します。`migrations/*.up.sql` は Postgres の初回起動時に
自動適用されます（ファイル名の昇順）。

### ローカル開発（ホットリロードあり）

DB だけ Compose で起動し、バックエンドとフロントエンドは手元で動かします。

```sh
# 1. DB を起動
docker compose up -d postgres

# 2. 環境変数を用意
cp .env.example .env

# 3. API サーバー（:8090）
go run ./cmd/server

# 4. フロントエンド（:5174、/api は API サーバーへプロキシ）
cd frontend && npm install && npm run dev
```

ブラウザで http://localhost:5174 を開きます。

Vite のプロキシ先ポートは `BACKEND_PORT` → `PORT` → `8080` の順に解決されるため、
`.env` の `PORT` を変える場合は同じ値が Vite 側にも渡るようにしてください。

## 環境変数

| 変数 | 必須 | 既定値 | 説明 |
| --- | --- | --- | --- |
| `DATABASE_URL` | ✓ | — | PostgreSQL 接続文字列 |
| `PORT` | | `8080` | API サーバーの待ち受けポート |

ローカルでは `.env` が読み込まれます（無くてもエラーにはなりません）。本番では実際の環境変数を使う想定です。

## マイグレーション

`migrations/` に `NNNN_name.up.sql` / `.down.sql` の組で置いています。Compose の初回起動以外で
適用する場合は psql で直接流してください。

```sh
psql "$DATABASE_URL" -f migrations/0004_manga_volume_progress.up.sql
```

新しいマイグレーションを追加したら、docker-compose.yml の postgres の volumes にも追記します。

## API

認証が必要なエンドポイント（`/api/records/*`）はセッション Cookie を送る必要があります。
それ以外はログイン不要です。

| メソッド | パス | 説明 |
| --- | --- | --- |
| `POST` | `/api/auth/signup` | サインアップ |
| `POST` | `/api/auth/login` | ログイン |
| `POST` | `/api/auth/logout` | ログアウト |
| `GET` | `/api/auth/me` | ログイン中のユーザー |
| `GET` | `/api/records?type=&status=&sort=` | ライブラリ一覧（`sort`: `default`/`title`/`score`/`updated`/`added`） |
| `POST` | `/api/records` | ライブラリに追加 / ステータス変更 |
| `PATCH` | `/api/records/:id` | 進捗・評価・メモ・ステータスの更新 |
| `DELETE` | `/api/records/:id` | ライブラリから削除 |
| `GET` | `/api/search?type=&q=` | 作品検索 |
| `GET` | `/api/anilist/media?type=&ids=` | ID 指定で作品情報を一括取得 |
| `GET` | `/api/anilist/relations?id=` | 関連作品 |
| `GET` | `/api/recommendations?type=&genres=` | ジャンル指定のおすすめ |
| `GET` | `/api/home/season-anime[?season=&year=]` | 今季アニメ（`season`/`year` 指定で任意シーズン） |
| `GET` | `/api/home/trending` | 人気アニメ |
| `GET` | `/api/home/trending-manga` | 人気漫画（Jikan でランキング・掲載誌を補完） |
| `POST` | `/api/translate` | テキストの機械翻訳 |

`/api/*` 以外のパスは `frontend/dist` の静的ファイルにフォールバックします（SPA ルーティング）。

## 開発コマンド

```sh
go test ./...          # バックエンドのテスト
gofmt -l .             # フォーマットチェック（CI と同じ）
go vet ./...
go build ./...

cd frontend
npm run lint           # oxlint
npm run build          # tsc -b && vite build
```

## 外部 API のレート制限

クライアント側でスロットリングを実装済みです。挙動を変える際は以下を踏まえてください。

- **AniList** — 目安 30 req/分。トークンバケット方式（バースト 5）で平均レートを抑え、
  429 を受けたら共有バケットの再開時刻を後ろ倒ししてリトライします。
- **Jikan** — 目安 3 req/秒。全呼び出しで最小リクエスト間隔 350ms を共有します。
- **MyMemory** — 匿名利用は 1 リクエスト約 500 バイトまで。長いあらすじは文単位に分割して翻訳します。
