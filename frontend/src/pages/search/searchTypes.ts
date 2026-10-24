// 検索・追加画面（案8）で使う型。
import type { MediaKind } from '../../lib/util'
import type { AniListItem } from '../../types'

export type { MediaKind }

/** 検索結果に並べる作品 1 件（AniList の結果をアプリ側で整えたもの） */
export interface WorkSummary {
  id: number
  title: string
  /** 表紙画像。無いときは頭文字の色ブロックで出す */
  coverUrl?: string | null
  /** 表示用のジャンル（日本語にしたもの）。先頭 2 つを出す */
  genres: string[]
  /** この作品を検索した種別。内容で探すでは文で種別が変わり、画面の種別と違うことがある */
  kind: MediaKind
  /** 作品モーダルに渡す元の検索結果 */
  source: AniListItem
}

/** AI が文から作った検索条件 */
export interface AiConditions {
  /** 実際に検索した種別 */
  kind: MediaKind
  /** 文に「漫画」などとあり、画面の種別と関係なく文から決めたとき true */
  kindFromText: boolean
  /** 日本語のジャンル名 */
  genres: string[]
  /** タグ名。和訳の表を持っていないので英語のまま */
  tags: string[]
  excludedGenres: string[]
  excludedTags: string[]
  /** 文に作品名などの固有名詞があったときだけ入る */
  query: string
}

/** 従来の作品名検索に落ちた理由。3 つは画面で出し分ける */
export type FallbackReason =
  | 'ai_unavailable' // AI が使えない
  | 'ai_failed' // AI が失敗した
  | 'no_conditions' // 文から条件を読み取れなかった

/** 内容で探す API の結果 */
export interface ContentSearchResult {
  works: WorkSummary[]
  /** 従来検索に落ちたときは null */
  conditions: AiConditions | null
  /** 従来検索に落ちたときだけ理由が入る */
  fallback: FallbackReason | null
}
