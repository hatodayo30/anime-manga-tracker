import { useCallback, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useWorkModal } from '../components/WorkModalContext'
import { useAuth } from '../context/AuthContext'
import { useKind } from '../hooks/useKind'
import { api } from '../lib/api'
import { STATUS_LABELS, containsKana, containsKanji, kanaToRomaji, translateGenre } from '../lib/util'
import type { MediaKind } from '../lib/util'
import type { AniListItem, LibraryRecord, NaturalSearchResult } from '../types'
import { SearchPage as SearchScreen } from './search/SearchPage'
import type { AiConditions, ContentSearchResult, FallbackReason, WorkSummary } from './search/searchTypes'

// 検索・追加画面。見た目と入力の扱いは search/SearchPage.tsx（案8）にあり、ここは API と
// アプリの状態（種別・ログイン・作品モーダル・ライブラリ）をつなぐだけ。

// 同じ作品（anilistId）を除いた上で、AniListの人気値（popularity）降順にまとめる。
// ひらがな/カタカナ入力時、元のかな検索とローマ字変換検索の両方を実行して結果をマージするために使う。
function mergeSearchResultsByPopularity(a: AniListItem[], b: AniListItem[]): AniListItem[] {
  const byId = new Map<number, AniListItem>()
  for (const item of [...a, ...b]) {
    if (!byId.has(item.anilistId)) byId.set(item.anilistId, item)
  }
  return Array.from(byId.values()).sort((x, y) => (y.popularity ?? 0) - (x.popularity ?? 0))
}

function toWorkSummary(item: AniListItem, kind: MediaKind): WorkSummary {
  return {
    id: item.anilistId,
    title: item.title,
    coverUrl: item.coverImageUrl || null,
    genres: (item.genres || []).map(translateGenre),
    kind,
    source: item,
  }
}

const FALLBACK_REASONS: Record<NonNullable<NaturalSearchResult['fallback']>, FallbackReason> = {
  unavailable: 'ai_unavailable',
  failed: 'ai_failed',
  no_conditions: 'no_conditions',
}

// requested は画面で選んでいた種別。AI は文に種別の指定が無くても種別を返すことがあり、それが画面の種別と
// 同じなら検索結果は変わらないので、「種別は文から」と出すのは画面の種別と違ったときだけにする。
function toAiConditions(result: NaturalSearchResult, requested: MediaKind): AiConditions | null {
  const c = result.conditions
  if (!c) return null
  return {
    kind: result.mediaType,
    kindFromText: !!c.mediaType && c.mediaType !== requested,
    genres: c.genres.map(translateGenre),
    tags: c.tags,
    excludedGenres: c.excludedGenres.map(translateGenre),
    excludedTags: c.excludedTags,
    query: c.query,
  }
}

type LibraryByKind = Partial<Record<MediaKind, Map<number, LibraryRecord>>>

export function SearchPage() {
  const [kind, setKind] = useKind()
  const { user, refresh } = useAuth()
  const navigate = useNavigate()
  const openWorkModal = useWorkModal()
  // 結果のバッジ（「見た ✓」など）用。内容で探すは文で種別が変わるので種別ごとに持つ
  const [library, setLibrary] = useState<LibraryByKind>({})

  const loadLibrary = useCallback(async (mediaType: MediaKind) => {
    const records = await api.listRecords({ type: mediaType })
    setLibrary((prev) => ({ ...prev, [mediaType]: new Map(records.map((r) => [r.anilistId, r])) }))
  }, [])

  // ライブラリの取得は中断しない（結果を捨てても、次の検索のバッジに使える）
  const searchByTitle = useCallback(
    async (q: string, mediaType: MediaKind, signal: AbortSignal) => {
      const romaji = containsKana(q) && !containsKanji(q) ? kanaToRomaji(q) : null
      const shouldMergeRomaji = !!romaji && romaji !== q
      const [searchResults, romajiResults] = await Promise.all([
        api.searchAniList({ type: mediaType, q, signal }),
        shouldMergeRomaji ? api.searchAniList({ type: mediaType, q: romaji, signal }) : Promise.resolve([]),
        loadLibrary(mediaType),
      ])
      const items = shouldMergeRomaji ? mergeSearchResultsByPopularity(searchResults, romajiResults) : searchResults
      return items.map((item) => toWorkSummary(item, mediaType))
    },
    [loadLibrary],
  )

  // AI の失敗はサーバー側で従来の検索に落ちて 200 が返る。ここで投げるのは、未ログイン（401）か
  // 従来の検索まで失敗したときだけ。ライブラリは結果の種別が決まってから引く（文で種別が変わりうるため）。
  const searchByContent = useCallback(
    async (q: string, mediaType: MediaKind, signal: AbortSignal): Promise<ContentSearchResult> => {
      let result: NaturalSearchResult
      try {
        result = await api.naturalSearch({ type: mediaType, q, signal })
      } catch (err) {
        // セッション切れ。ログイン状態を取り直すと ProtectedRoute がログイン画面へ送る
        if (err instanceof Error && err.message === 'login required') void refresh()
        throw err
      }
      await loadLibrary(result.mediaType)
      return {
        works: result.items.map((item) => toWorkSummary(item, result.mediaType)),
        conditions: toAiConditions(result, mediaType),
        fallback: result.fallback ? FALLBACK_REASONS[result.fallback] : null,
      }
    },
    [loadLibrary, refresh],
  )

  const statusLabelFor = (work: WorkSummary) => {
    const record = library[work.kind]?.get(work.id)
    return record ? STATUS_LABELS[work.kind][record.status] : null
  }

  const openWork = (work: WorkSummary) => {
    openWorkModal({
      item: work.source,
      mediaType: work.kind,
      record: library[work.kind]?.get(work.id) ?? null,
      // 追加・変更したらバッジを直す。検索はやり直さない（AI をもう一度呼ばないため）
      onChange: () => void loadLibrary(work.kind).catch(() => {}),
    })
  }

  return (
    <SearchScreen
      kind={kind}
      onKindChange={setKind}
      isLoggedIn={!!user}
      onLoginClick={() => navigate('/login?next=%2Fsearch')}
      searchByTitle={searchByTitle}
      searchByContent={searchByContent}
      onOpenWork={openWork}
      statusLabelFor={statusLabelFor}
    />
  )
}
