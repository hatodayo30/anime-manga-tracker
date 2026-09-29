import type { MediaKind, ProgressUnit, Status } from './lib/util'

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
  progressUnit: ProgressUnit
  nextAiringAt: string | null
  rating: number | null
  memo: string
  createdAt: string
  updatedAt: string
}

// AniList検索結果（search/recommendations/season-anime/trending で共通のshape）。
export interface AniListItem {
  anilistId: number
  title: string
  coverImageUrl: string
  genres: string[]
  total: number | null // アニメ=話数（episodes）/ 漫画=話数（chapters）
  volumes?: number | null // 漫画の既刊巻数。巻数単位で記録している作品の総数に使う
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
  progressUnit: ProgressUnit
  nextAiringAt: number | null
}

export interface UpdateRecordInput {
  status?: Status
  progress?: number
  // AniListの最新情報で総数がズレていたときの同期用。
  total?: number
  // 単位を切り替えるときは progress / total も新しい単位の値に揃えて一緒に送る。
  progressUnit?: ProgressUnit
  // 1〜5で評価をセット、0で未評価に戻す。
  rating?: number
  memo?: string
}
