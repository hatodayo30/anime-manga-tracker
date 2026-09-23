import type {
  AniListItem,
  LibraryRecord,
  Me,
  MangaRankingItem,
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
  listRecords: ({ type = '', status = '' }: { type?: string; status?: string } = {}) =>
    request<LibraryRecord[]>('/records' + buildQuery({ type, status })),
  createRecord: (body: NewRecordInput) =>
    request<LibraryRecord>('/records', { method: 'POST', body: JSON.stringify(body) }),
  updateRecord: (id: number, body: UpdateRecordInput) =>
    request<LibraryRecord>(`/records/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deleteRecord: (id: number) => request<null>(`/records/${id}`, { method: 'DELETE' }),

  searchAniList: ({ type, q = '' }: { type: MediaKind; q?: string }) =>
    request<AniListItem[]>('/search' + buildQuery({ type, q })),
  mediaByIds: ({ type, ids }: { type: MediaKind; ids: number[] }) => {
    if (ids.length === 0) return Promise.resolve<AniListItem[]>([])
    return request<AniListItem[]>('/anilist/media' + buildQuery({ type, ids: ids.join(',') }))
  },
  relations: ({ id }: { id: number }) => request<RelatedWork[]>('/anilist/relations' + buildQuery({ id: String(id) })),
  recommendations: ({ type, genres }: { type: MediaKind; genres: string[] }) => {
    if (genres.length === 0) return Promise.resolve<AniListItem[]>([])
    return request<AniListItem[]>('/recommendations' + buildQuery({ type, genres: genres.join(',') }))
  },
  translate: async ({ text, target = 'ja' }: { text: string; target?: string }) => {
    if (!text) return ''
    const { translated } = await request<{ translated: string }>('/translate', {
      method: 'POST',
      body: JSON.stringify({ text, target }),
    })
    return translated
  },

  seasonAnime: ({ season, year }: { season?: string; year?: number } = {}) =>
    request<AniListItem[]>('/home/season-anime' + buildQuery({ season, year: year ? String(year) : undefined })),
  trending: () => request<AniListItem[]>('/home/trending'),
  trendingManga: () => request<MangaRankingItem[]>('/home/trending-manga'),

  getMe: () => request<Me>('/auth/me'),
  login: (body: { email: string; password: string }) =>
    request<Me>('/auth/login', { method: 'POST', body: JSON.stringify(body) }),
  signup: (body: { email: string; password: string }) =>
    request<Me>('/auth/signup', { method: 'POST', body: JSON.stringify(body) }),
  logout: () => request<null>('/auth/logout', { method: 'POST' }),
}
