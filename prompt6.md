シーズン画面（/season）の作品取得が遅い問題を調査し、パフォーマンスを改善してください。

## 症状

- シーズン画面で年・シーズンを切り替えると、`GET /api/home/season-anime?season=...&year=...` の応答に数秒（DevTools計測で2〜4.5秒程度）かかる。
- 同じ season/year に何度切り替えても毎回同じだけ待たされる（キャッシュが効いている様子がない）。

## 調査してほしいこと（すでに分かっている手がかり）

1. `internal/anilist/client.go` の `Client` は、全メソッド共通でグローバルな `minRequestInterval = 2100ms` のスロットル（`mu sync.Mutex` + `lastCall`）を持っている。AniListへの呼び出しが短時間に重なると、後続の呼び出しは直前の呼び出しから2.1秒経つまで直列に待たされる。
   - これがAniList公式のレート制限（目安30req/分）を守るために必要な仕組みなのか、現状の間隔設定が必要以上に保守的なのかを確認してほしい。
2. `internal/usecase/home/home.go` を見ると、ホーム画面向けの `CurrentSeasonAnime`（season/year省略時）は `seasonAnimeCache`（10分TTL）でキャッシュされているが、シーズンブラウジング画面向けの `SeasonAnimeFor`（season/year指定あり）は素通りで毎回AniListに問い合わせている（`home.go` の `CurrentSeasonAnime` と `SeasonAnimeFor` の実装差分を参照）。
3. 開発環境（Vite dev server + React StrictMode）では `SeasonPage.tsx` の `useEffect` が二重発火し、`season-anime` リクエストが2本連続で飛ぶ可能性がある。2本目が上記グローバルスロットルに引っかかり、体感の遅さを悪化させていないか確認してほしい（本番ビルドでも再現するかどうかも見てほしい）。

## 改善の方向性（たたき台、調査結果次第で修正してよい）

- `SeasonAnimeFor` にもキャッシュを追加する。season/yearの組み合わせは無数にあるので、`CurrentSeasonAnime` と同じ単一キャッシュではなく、`season+year` をキーにしたキャッシュ（TTL・最大エントリ数を検討）にする。
- AniListのレート制限の実態を踏まえて `minRequestInterval` の見直し余地がないか検討する（ただし429を誘発しないよう慎重に）。
- フロント側で同一 season/year への不要な二重リクエストが発生していないか確認し、必要なら防ぐ。

## 進め方

まず `internal/anilist/client.go`・`internal/usecase/home/`・`frontend/src/pages/SeasonPage.tsx` を調査し、実際のボトルネックがどこにあるか（スロットル起因か、キャッシュ欠如起因か、両方か）を特定してください。
その上で改善方針（キャッシュ設計・スロットル設定・フロント側修正の要否）を提案し、合意が取れてから実装に進めてください。
