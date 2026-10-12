# Orca で並列エージェントを動かす

複数の AI エージェントを git worktree に分けて同時に走らせるための手順書。
Orca の使い方と、このリポジトリ固有の制約を 1 枚にまとめている。

最終更新: 2026-10-12

---

## 要点

**コードを書くエージェントは好きなだけ並列化してよいが、サーバーを起動するのは 1 本だけ。**

このリポジトリはキャッシュ・レート制限・定期ジョブがすべてプロセス内にあり、
サーバーを 1 つ起動するだけで AniList を 5 本・Jikan を 10 本消費する。
AniList の目安は 30 req/分なので、5 ワークツリーで同時起動すれば即座に 429 を踏む。
詳細は [ARCHITECTURE.md](ARCHITECTURE.md) の「定期ジョブ」と「外部 API 連携」を参照。

---

## 前提

### どの Orca か

**同名の別プロジェクトが 2 つある。** 使うのは前者。

| | onorca.dev ← これ | itsorca.dev |
| --- | --- | --- |
| 作者 | Stably AI | Orca Labs (vcashwin) |
| repo | `stablyai/orca` | `vcashwin/orca-releases` |
| 分離方式 | git worktree | コンテナ |
| 対応 OS | macOS / Windows / Linux | macOS のみ |
| ライセンス | MIT | — |

### インストール

```sh
brew install --cask stablyai/orca/orca
```

DMG は [onorca.dev/download](https://www.onorca.dev/download)。Arch なら `yay -S stably-orca-bin`。
Claude のサブスクをそのまま使うので API キーの用意は不要。

---

## 最初の 1 回だけ

### 1. Postgres を 1 つだけ起動する

**メインのチェックアウトから**起動する。ワークツリーごとに `docker compose up` すると、
Compose のプロジェクト名がディレクトリ名から決まるために別々の postgres コンテナが立ち、
全部がホストの 5432 を取り合って失敗する。

```sh
docker compose up -d postgres
```

全ワークツリーがこの 1 つを `localhost:5432` で共有する。

### 2. ランタイムを起動してリポジトリを登録する

CLI は Orca アプリのランタイムを経由するので、アプリが起動していないと
`runtime_unavailable` で失敗する。

```sh
orca open                              # アプリを起動してランタイムが繋がるまで待つ
orca repo add --path .                 # このリポジトリを Orca に登録
orca repo show --repo path:$PWD --json # 登録されたか確認
```

**リポジトリセレクタは `id:<id>` / `name:<name>` / `path:<path>` の 3 形式。**
`<selector>` のようなプレースホルダをそのまま打つと zsh が `<` をリダイレクトと解釈して
`parse error near '\n'` になる。

### 3. `orca.yaml` を主ブランチに置く

リポジトリルートの `orca.yaml` と `.worktreeinclude` が設定を担うが、
**Orca は主ブランチ（main）の `orca.yaml` を見る。** 作業ブランチに置いたままでは効かないので、
main にマージしておくこと。

設定が効いているかは、使い捨てのワークツリーを作って setup の出力を見るのが確実。

```sh
orca worktree create --name tmp-check --setup run --activate --json
```

`このワークツリーのポート: 8091` のような出力が出れば `orca.yaml` の setup が走っている。
確認したら破棄してよい。

> `orca repo hooks show` / `orca repo hooks set` は未リリースの CLI コマンド。
> v1.4.224 には存在しない（`orca repo` は list / add / show / set / set-base-ref / search-refs のみ）。
> GUI 側は Settings → Repository → Worktree Hooks。

---

## 日常の流れ

1. **タスクを作る** — サイドバーから新しいワークツリーを作る。名前はブランチ名になる。
2. **setup が走るのを待つ** — `.env` のコピー、ポート割り当て、依存の確認が自動で走る。
   `setupAgentStartupPolicy: wait-for-setup` にしてあるので、エージェントはこれが終わるまで待つ。
3. **エージェントを起動する** — Claude Code / Codex / Cursor CLI などを選び、プロンプトを渡す。
   ワークツリーごとに独立したターミナル・コンテキスト・ブラウザタブを持つ。
4. **並行して他のタスクを立てる** — サイドバーで行き来する。使用量メーターがステータスバーに出る。
5. **diff ビューでレビューする** — ワークツリー単位で差分を読む。
6. **マージする** — 通ったものから取り込む。

CLI からも同じことができる。

```sh
orca worktree create --name fix-sort-order --agent claude --prompt "<指示>" --json
orca worktree list --json
orca agent launch --agent claude --worktree id:<repoId>::<path> --prompt "<指示>" --json
```

ワークツリー ID は `<repoId>::<worktreePath>` 形式。`worktree create --json` の出力から丸ごとコピーする。

---

## このリポジトリで守ること

### ルール 1: サーバーを起動するのは 1 ワークツリーだけ

動作確認用に 1 つだけ常駐させ、他のワークツリーは次の 2 つまでに留める。

```sh
go test ./...
go build ./...
```

usecase のテストはフェイクを差し込むので外部 API を一切叩かない。
これなら何本並列に走らせてもレートを消費しない。

`orca.yaml` の `defaultTabs` に**意図的にサーバー起動タブを置いていない**のはこのため。

### ルール 2: `migrations/` を触るタスクは並列から外す

DB は全ワークツリーで共有している。エージェントがマイグレーションを足すと他にも即座に効く。
マイグレーションランナーは `pg_advisory_lock` を掛けるので同時適用では壊れないが、
データレベルの衝突は防げない（`0004` のような破壊的なデータ移行がある）。

### ルール 3: `package.json` を触るタスクも並列から外す

`frontend/node_modules` を `worktree.sharedDirectories` で共有して
`npm install` の待ち時間を省いている。依存を変えるタスクを並列で走らせるときは、
`orca.yaml` の `sharedDirectories` から該当行を外すこと。

---

## 自動化されている部分

### `.worktreeinclude`

gitignore 済みファイルを新しいワークツリーへ**コピー**する（symlink ではない）。

現在は `.env` のみ。`.env` が無いと `config.Load` が `DATABASE_URL is required` で
起動時にエラーになるため、これが無いと何も動かない。

1 行 1 パス、リポジトリルート基準の literal パスのみ。glob と除外指定は無視される。

### `orca.yaml` の setup スクリプト

ワークツリー作成直後に、そのワークツリー内で走る。

1. `.env` が無ければ `.env.example` からコピー
2. **空いている API ポートを探して `.env` の `PORT` を書き換える**
3. `frontend/node_modules` が無ければ `npm install`
4. 割り当てたポートと起動コマンドを表示

ポートをずらすのは、`.env` をコピーすると全ワークツリーが `PORT=8090` になり、
サーバーを起動したとき衝突するため。8091 から順に空きを探す。

> `sed -i ''` は macOS (BSD sed) の書き方。Linux で使うなら `sed -i` に直すこと。

---

## 手動でサーバーを起動するとき

setup が表示したポートを使う。`N` は割り当てられた API ポート（8091, 8092, ...）。

```sh
# API
go run ./cmd/server

# フロント（別ターミナル）
cd frontend && BACKEND_PORT=N npm run dev -- --port $((N + 100))
```

`BACKEND_PORT` の明示が必要なのは、[vite.config.ts](../frontend/vite.config.ts) が
`process.env` を直接読んでいて、リポジトリルートの `.env` を読み込まないため。
`--port` が必要なのは、Vite の `5174` がハードコードで環境変数を見ていないため。

| 変数 | 解決順 | 備考 |
| --- | --- | --- |
| Go の待ち受けポート | `.env` の `PORT` → `8080` | setup が書き換える |
| Vite のプロキシ先 | `BACKEND_PORT` → `PORT` → `8080` | シェルの環境変数のみ |
| Vite の待ち受けポート | `--port` → `5174` | ハードコード |

---

## 困ったとき

| 症状 | 原因 |
| --- | --- |
| `DATABASE_URL is required` で即死 | `.env` がコピーされていない。`.worktreeinclude` が効いているか確認 |
| `address already in use` | 別のワークツリーでサーバーが動いている。ルール 1 を思い出す |
| AniList が 429 を返す | サーバーを複数起動している。全部落として数分待つ |
| フロントが API に繋がらない | `BACKEND_PORT` が `.env` の `PORT` とずれている |
| postgres が起動しない | ワークツリーから `docker compose up` した。メインのチェックアウトで 1 つだけ起動する |
| `npm install` が毎回走る | `sharedDirectories` が効いていない。`orca.yaml` が main にあるか確認 |
| `runtime_unavailable` | Orca アプリが起動していない。`orca open` を先に実行する |
| `zsh: parse error near '\n'` | `<selector>` をそのまま打っている。`path:$PWD` などに置き換える |
| setup が走らない | `orca.yaml` が作業ブランチにしか無い。main にマージする |
