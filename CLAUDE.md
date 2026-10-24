# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

アニメ・漫画の視聴記録アプリ。Go (Echo) のシングルバイナリが API と React 製フロントエンドの
静的ファイルを両方配信する。作品情報は AniList / Jikan / MyMemory から取得する。

**コメント・コミットメッセージ・ドキュメントはすべて日本語で書く。**

---

## コマンド

```sh
go test ./...          # バックエンド全体
go test ./internal/usecase/airing -run TestSync   # 1 パッケージ / 1 テスト
gofmt -l .             # CI と同じフォーマットチェック（出力が空なら OK）
go vet ./...
go build ./...

cd frontend
npm run lint           # oxlint
npm run build          # tsc -b && vite build（型チェックを含む）
```

**CI は `go test` を回していない**（gofmt / vet / build と、フロントの lint / build のみ）。
Go のコードを変えたら必ず手元で `go test ./...` を実行すること。

サーバーを起動する必要がある場合は「外部 API のレート」の節を先に読むこと。

---

## アーキテクチャ

3 層 + 依存性逆転。`handler` → `usecase` → `repository` / `gateway` で、依存は常に内側を向く。

**usecase 層が自分でインターフェースを定義し、`infrastructure/postgres` と外部 API
クライアント（`anilist` / `jikan` / `translate`）がそれを実装する。**
usecase は具体的な DB や API を知らない。新しい外部依存を足すときもこの向きを保つこと。

レイヤ構成・データモデル・画面ごとの処理フローの詳細は **`docs/ARCHITECTURE.md`**。

---

## 壊してはいけない不変条件

コードを読んだだけでは気付きにくく、壊すと別の場所が壊れるもの。

### 外部 API のレート

- AniList のトークンバケットは **プロセス全体で 24 req/分**（`internal/anilist/client.go`）。
  全呼び出しで 1 つのバケットを共有する。
- **サーバーを起動すると即座に `home-cache-warm` が走る**（AniList 3 本 + Jikan 10 本）。
  続けて `next-airing-sync`（AniList 最大 2 本）。
- 複数のワークツリーで並列に作業しているときは **サーバーを起動しない**。
  検証は `go test ./...` と `go build ./...` まで（usecase のテストはフェイクを使うので
  外部 API を叩かない）。

### プロセス内で完結している

キャッシュ・single-flight・トークンバケット・定期ジョブはすべて 1 プロセスのメモリ上にある。
**このアプリは水平スケールできない**（2 タスク並走で実効レートが倍になり 429 を踏む）。
ECS は `desiredCount: 1` 固定。この前提を崩す変更は BACKLOG で設計判断を確認してから。

### データの扱い

- **`records.progress` / `total` の単位は `media_type` で決まる** — アニメは話数、漫画は巻数。
  専用カラムを足さない設計。
- **`updated_at` は利用者の操作でだけ更新する。** 定期ジョブ（`usecase/airing`）は意図的に
  触らない。更新するとライブラリの「最近更新した順」が定期ジョブで塗り替わる。
- **ユーザースコープはリポジトリの SQL で強制する** — `WHERE ... AND user_id = $N`。
  他人のレコードは `ErrNotFound` にし、存在しないのか他人のものなのかを区別しない。
  例外は `airing.Repository` と `housekeeping.Repository` で、定期ジョブが保守のために全ユーザーを横断する。
  **ユーザースコープされない操作は、それと分かるようインターフェースを分ける。**
- **`ORDER BY` 句は SQL に直接埋め込んでいる**（`orderByClauses` map）。
  map に定義済みのキー以外は絶対にクエリへ渡さない。

### 失敗の扱い

補完的な情報の失敗は本体を止めない、という方針が各所にある。Jikan のエンリッチが失敗しても
ランキングは返す、検索のフォールバックが失敗しても AniList の結果は返す、など。
この方針を変えるときは意図的に行うこと。

---

## 判断の理由がどこにあるか

このリポジトリは **「なぜそうしたか」をコードコメントと `BACKLOG.md` に蓄積している。**
実装を変えるときは、その判断を上書きしていないか先に確認すること。

| 場所 | 内容 |
| --- | --- |
| `BACKLOG.md` | **残タスクの唯一の正。** 完了済みの項目にも「決めたこと」「残っている穴」が書いてある |
| `docs/ARCHITECTURE.md` | 実装と処理フローの全体像 |
| `docs/DEPLOY-AWS.md` / `-PHASE2.md` | AWS デプロイの手順 |
| `docs/PARALLEL-AGENTS.md` | 複数エージェントを並列で動かすときの手順と制約 |
| `prompt*.md` | 書かれた時点のスナップショット。**着手順は BACKLOG を優先する** |

コメントは「何をしているか」ではなく **「なぜそうしたか」「なぜそうしなかったか」** を書く。
既存のコメントがこの密度なので、それに合わせること。

---

## 規約

### Git

- **main / master では編集もコミットもできない**（`.claude/hooks/block-main-branch.sh` が
  PreToolUse で弾く）。先に `git checkout -b <type>/<short-description>` すること。
- **AWS を変更する操作と、Postgres ボリュームを消す操作はブロックされる**
  （`.claude/hooks/block-destructive.sh`）。`aws` は `describe-*` / `list-*` / `get-*` のみ通る。
  ECS / RDS の操作は `docs/DEPLOY-AWS.md` を見ながら人間が実行する。
- **認証情報ファイル（`~/.aws` / `~/.ssh` / `~/.config/gh` など）は読めない**
  （`.claude/hooks/block-secret-reads.sh` と `permissions.deny`）。
  このリポジトリの作業に認証情報は要らない。リポジトリ内の `.env` は読める。
- フックを変更したら回帰テストを通すこと。
  `bash .claude/hooks/block-destructive.test.sh` / `bash .claude/hooks/block-secret-reads.test.sh` /
  `bash .claude/hooks/block-main-branch.test.sh`
- コミットメッセージは Conventional Commits 形式（`feat:` / `fix:` / `docs:` / `chore:`）＋日本語。
- commit・push・PR 作成は `/pr` でまとめて実行できる。

### テスト

- **外部テストパッケージ**（`package airing_test`）に置き、公開 API 経由で検証する。
- モックライブラリは使わず、**usecase が定義したインターフェースを満たす fake を手書きする**
  （`fakeRepo` / `fakeAniList` など）。呼び出し回数や渡された引数をフィールドに記録して検証する。
- 各テストに「何を保証しているか」を日本語のコメントで書く。

### マイグレーション

- `migrations/NNNN_name.up.sql` / `.down.sql` を連番で置くだけ。`embed.FS` に焼き込まれ、
  起動時に未適用のものだけが適用される。
- `golang-migrate` は意図的に入れていない（`database/sql` 前提で `pgxpool` と別に接続を張るため）。
- ロールバックは自動では行わない。

---

## 複数エージェントで並列作業するとき

`orca.yaml` / `.worktreeinclude` がワークツリーごとの `.env` コピーとポート割り当てを自動化している。
手順は `docs/PARALLEL-AGENTS.md`。守ること:

- **サーバーを起動するのは 1 ワークツリーだけ**（上記「外部 API のレート」）
- **`migrations/` を触るタスクは並列に入れない** — DB は全ワークツリーで共有している
- **`package.json` を触るタスクも並列に入れない** — `frontend/node_modules` は symlink で共有しており、
  片方の `npm install` が即座に全ワークツリーに効く
