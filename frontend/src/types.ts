import type { MediaKind, Status } from './lib/util'

export interface Me {
  id: number
  email: string
  createdAt: string
}

// GET /api/records の各要素。nextAiringAtはサーバーでRFC3339文字列として返る。
export interface LibraryRecord {
  id: number
  anilistId: number
  mediaType: MediaKind
  title: string
  coverImageUrl: string
  genres: string[]
  status: Status
  progress: number
  total: number | null
  nextAiringAt: string | null
  rating: number | null
  memo: string
  createdAt: string
  updatedAt: string
}

// AniList検索結果（search/recommendations/season-anime/trending で共通のshape）。
export interface AniListItem {
  anilistId: number
  title: string // 日本語 UI 向けの作品名。ライブラリへ追加するときはこちらを保存する
  titleEn?: string // EN 表示を選んだときの作品名（BACKLOG 6）。表示にだけ使う
  coverImageUrl: string
  genres: string[]
  total: number | null // アニメ=話数 / 漫画=巻数（進捗の追跡単位）
  volumes?: number | null
  score?: number | null // 0-100
  synopsis?: string
  nextAiringAt?: number | null // unix秒（Recordのnextairingatとは形式が異なる）
  nextEpisode?: number | null
  airingStatus?: string
  popularity?: number
  malId?: number | null
}

// GET /api/home/trending-manga の各要素（AniListItem + MAL由来の付加情報）。
export interface MangaRankingItem extends AniListItem {
  malRank?: number | null
  members?: number | null
  magazines: string[]
}

export interface RelatedWork {
  anilistId: number
  title: string
  titleEn?: string
  coverImageUrl: string
  mediaType: MediaKind
  relationType: string
}

// POST /api/records のリクエストボディ。
export interface NewRecordInput {
  anilistId: number
  mediaType: MediaKind
  title: string
  coverImageUrl: string
  genres: string[]
  total: number | null
  status: Status
  nextAiringAt: number | null
}

export interface UpdateRecordInput {
  status?: Status
  progress?: number
  // AniListの最新情報で総数（アニメ=話数 / 漫画=巻数）がズレていたときの同期用。
  total?: number
  // 1〜5で評価をセット、0で未評価に戻す。
  rating?: number
  memo?: string
}

// GET /api/search/natural（自然文検索、BACKLOG 9-b）で AI が作った検索条件。
// genres / tags は AniList の英語名のまま。mediaType は文で種別を指定したときだけ入る。
export interface NaturalSearchConditions {
  mediaType?: MediaKind
  genres: string[]
  tags: string[]
  excludedGenres: string[]
  excludedTags: string[]
  query: string
}

// GET /api/search/natural の応答。AI が使えないときも 200 で従来の検索結果が返り、fallback に理由が入る。
// mediaType は実際に検索した種別で、画面で選んでいる種別と違うことがある。
export interface NaturalSearchResult {
  mediaType: MediaKind
  conditions: NaturalSearchConditions | null
  fallback?: 'unavailable' | 'failed' | 'no_conditions'
  items: AniListItem[]
}
