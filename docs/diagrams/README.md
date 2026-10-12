# 構成図

| ファイル | 内容 |
| --- | --- |
| `aws-architecture.drawio` | AWS の構成図。**Phase 1（ALB なし）** と **Phase 2（ALB / HTTPS）** の 2 ページ |

draw.io（diagrams.net）形式。AWS 公式のアイコンセット（draw.io の `mxgraph.aws4` シェイプ）を使っている。
XML を圧縮せずそのまま保存しているので、差分がレビューできる。

## 開く・編集する

**インストールは不要。** 次のどれかで開ける。

1. **ブラウザ** — <https://app.diagrams.net> を開き、`Open Existing Diagram` から
   `aws-architecture.drawio` を選ぶ（ファイルをウィンドウにドラッグしてもよい）。
2. **VS Code** — 拡張機能 `hediet.vscode-drawio`（Draw.io Integration）を入れると、
   `.drawio` ファイルをエディタ内でそのまま編集できる。
3. **デスクトップアプリ** — `brew install --cask drawio`

編集したら上書き保存し、コミットする。

## PNG に書き出す

ドキュメントへの埋め込み用。ページごとに 1 枚ずつ書き出す。

1. 図を開き、下部のタブで書き出したいページ（`Phase 1` / `Phase 2`）を選ぶ
2. `File` → `Export as` → `PNG...`
3. 設定:
   - **Zoom: 200%**（Retina 表示でぼやけないように）
   - **Border Width: 10**
   - **Selection Only: オフ**
   - **Appearance: Light**（GitHub のダークモードでも読めるよう、背景を白で固定する）
   - **Transparent Background: オフ**
4. ファイル名は `aws-architecture-phase1.png` / `aws-architecture-phase2.png` としてこのディレクトリに保存

書き出したら [`../DEPLOY-AWS.md`](../DEPLOY-AWS.md) の簡易版（ASCII）の図を
`![Phase 1 構成図](diagrams/aws-architecture-phase1.png)` に差し替える。

## 図を変更したときの注意

- **アイコンを追加するときは、左サイドバーの AWS19 シェイプから貼ること。**
  スタイル文字列を手書きすると、存在しないシェイプ名（`mxgraph.aws4.application_load_balancer` など）を
  書いてしまい、draw.io 上では空白で表示される。
- **構成を変えたら `../DEPLOY-AWS.md` の手順本文も合わせて直す。**
  図だけ新しくなっている状態が一番ややこしい。
