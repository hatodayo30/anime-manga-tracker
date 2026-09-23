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
  createdAt: string
  updatedAt: string
}

// AniList検索結果（search/recommendations/season-anime/trending で共通のshape）。
export interface AniListItem {
  anilistId: number
  title: string
  coverImageUrl: string
  genres: string[]
  total: number | null
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
}
