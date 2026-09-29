# frontend

anime-manga-tracker のフロントエンド（React 19 + TypeScript + Vite）です。
セットアップ・全体構成はリポジトリルートの [README](../README.md) を参照してください。

## コマンド

```sh
npm install
npm run dev      # 開発サーバー（:5174、/api は Go サーバーへプロキシ）
npm run lint     # oxlint
npm run build    # tsc -b && vite build → dist/
npm run preview  # ビルド結果のプレビュー
```

`npm run dev` は `/api` を `http://localhost:${BACKEND_PORT || PORT || 8080}` にプロキシします。
Go サーバーのポートを変えている場合は同じ値を渡してください。

本番では `dist/` を Go サーバーが静的配信します（Dockerfile のマルチステージビルドで生成）。

## 構成

```
src/
  pages/       画面（Home / Search / Library / Recommend / Season / AuthForm）
  components/  Layout, PosterCard, WorkModal, KindToggle, ProtectedRoute など
  context/     AuthContext（ログイン状態）
  hooks/       useKind（アニメ / 漫画の切り替え）
  lib/         api.ts（APIクライアント）, util.ts, safeNextPath.ts
  types.ts     APIレスポンスの型
```

`WorkModal` は作品の詳細（あらすじの日本語訳・関連作品・進捗編集）を担当し、
`WorkModalContext` 経由でどの画面からでも開けます。
