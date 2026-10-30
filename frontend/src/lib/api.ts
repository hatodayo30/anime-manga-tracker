import type {
  AniListItem,
  LibraryRecord,
  Me,
  MangaRankingItem,
  LatestVolumeResult,
  NewReleasesResult,
  NewSeriesItem,
  NaturalSearchResult,
  NewRecordInput,
  RelatedWork,
  UpdateRecordInput,
} from '../types'
import type { MediaKind } from './util'

const API_BASE = '/api'

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(API_BASE + path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `request failed: ${res.status}`)
  }
  if (res.status === 204) return null as T
  return res.json()
}

function buildQuery(params: Record<string, string | undefined>): string {
  const usp = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value) usp.set(key, value)
  }
  const qs = usp.toString()
  return qs ? `?${qs}` : ''
}

export const api = {
  listRecords: ({ type = '', status = '', sort = '' }: { type?: string; status?: string; sort?: string } = {}) =>
    request<LibraryRecord[]>('/records' + buildQuery({ type, status, sort })),
  createRecord: (body: NewRecordInput) =>
    request<LibraryRecord>('/records', { method: 'POST', body: JSON.stringify(body) }),
  updateRecord: (id: number, body: UpdateRecordInput) =>
    request<LibraryRecord>(`/records/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deleteRecord: (id: number) => request<null>(`/records/${id}`, { method: 'DELETE' }),

  searchAniList: ({ type, q = '', signal }: { type: MediaKind; q?: string; signal?: AbortSignal }) =>
    request<AniListItem[]>('/search' + buildQuery({ type, q }), { signal }),
  mediaByIds: ({ type, ids }: { type: MediaKind; ids: number[] }) => {
    if (ids.length === 0) return Promise.resolve<AniListItem[]>([])
    return request<AniListItem[]>('/anilist/media' + buildQuery({ type, ids: ids.join(',') }))
  },
  relations: ({ id }: { id: number }) => request<RelatedWork[]>('/anilist/relations' + buildQuery({ id: String(id) })),
  recommendations: ({ type, genres }: { type: MediaKind; genres: string[] }) => {
    if (genres.length === 0) return Promise.resolve<AniListItem[]>([])
    return request<AniListItem[]>('/recommendations' + buildQuery({ type, genres: genres.join(',') }))
  },
  // 自然文検索（BACKLOG 9-b）。ログイン必須で、未ログインは 401（'login required'）になる。
  naturalSearch: ({ type, q, signal }: { type: MediaKind; q: string; signal?: AbortSignal }) =>
    request<NaturalSearchResult>('/search/natural' + buildQuery({ type, q }), { signal }),
  translate: async ({ text, title, target = 'ja' }: { text: string; title?: string; target?: string }) => {
    if (!text) return ''
    const { translated } = await request<{ translated: string }>('/translate', {
      method: 'POST',
      body: JSON.stringify({ text, title, target }),
    })
    return translated
  },

  seasonAnime: ({ season, year }: { season?: string; year?: number } = {}) =>
    request<AniListItem[]>('/home/season-anime' + buildQuery({ season, year: year ? String(year) : undefined })),
  trending: () => request<AniListItem[]>('/home/trending'),
  trendingManga: () => request<MangaRankingItem[]>('/home/trending-manga'),
  newSeries: () => request<NewSeriesItem[]>('/home/new-series'),
  newReleases: () => request<NewReleasesResult>('/new-releases/comics'),
  latestVolume: (title: string) => request<LatestVolumeResult>('/books/latest-volume' + buildQuery({ title })),

  getMe: () => request<Me>('/auth/me'),
  // 使えるログインの手段（BACKLOG 8-b）。Google は GOOGLE_CLIENT_ID などが無い環境では false
  authProviders: () => request<{ google: boolean }>('/auth/providers'),
  login: (body: { email: string; password: string }) =>
    request<Me>('/auth/login', { method: 'POST', body: JSON.stringify(body) }),
  signup: (body: { email: string; password: string }) =>
    request<Me>('/auth/signup', { method: 'POST', body: JSON.stringify(body) }),
  logout: () => request<null>('/auth/logout', { method: 'POST' }),
  // 成功すると、いまの端末以外のセッションはすべて消える。現在のパスワードが違えば 403
  changePassword: (body: { currentPassword: string; newPassword: string }) =>
    request<null>('/auth/password', { method: 'POST', body: JSON.stringify(body) }),

  // ホームの「AI のおすすめ」（BACKLOG 9-c、ログイン必須）。source が 'genre' のときは記録が少ないか
  // AI が使えなかったときのジャンルベースのおすすめで、reason は付かない。
  aiRecommendations: ({ type }: { type: MediaKind }) =>
    request<{ source: 'ai' | 'genre'; items: (AniListItem & { reason?: string })[] }>('/recommendations/ai' + buildQuery({ type })),
}
