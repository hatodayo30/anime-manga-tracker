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
  title: string
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
// GET /api/home/new-series（「新着」タブの新連載）の 1 件。startedOn は連載の開始日（"2026-10-23"）
export interface NewSeriesItem extends AniListItem {
  startedOn: string
}

// GET /api/new-releases/comics（「新着」タブの新刊、楽天ブックス）の 1 冊。salesDate は "2026-10-24"（JST）
export interface NewReleaseItem {
  title: string
  seriesName: string
  author: string
  publisherName: string
  isbn: string
  salesDate: string
  // 楽天の表記が「〜日頃」だった（日付は未確定）
  salesDateApproximate: boolean
  price: number
  url: string
  imageUrl: string
}

// configured が false のときは楽天のキーが無い環境で、from 以下は無く items は空
export interface NewReleasesResult {
  configured: boolean
  from?: string
  to?: string
  fetchedAt?: string
  // 期間内を読み切れなかった（一部だけ）
  truncated: boolean
  items: NewReleaseItem[]
}

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
  // character は文に出てきたキャラクター名（書かれたとおり）。名前が合うキャラクターが AniList に
  // いなかったときは空になり、名前は query に回る。characterNames は AniList で探した名前の候補（ローマ字など）
  character: string
  characterNames: string[] | null
}

// GET /api/search/natural の応答。AI が使えないときも 200 で従来の検索結果が返り、fallback に理由が入る。
// mediaType は実際に検索した種別で、画面で選んでいる種別と違うことがある。
export interface NaturalSearchResult {
  mediaType: MediaKind
  conditions: NaturalSearchConditions | null
  fallback?: 'unavailable' | 'failed' | 'no_conditions'
  items: AniListItem[]
}
