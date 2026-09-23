import { useEffect, useRef, useState } from 'react'
import { KindToggle } from '../components/KindToggle'
import { PosterCard } from '../components/PosterCard'
import { Thumb } from '../components/Thumb'
import { useWorkModal } from '../components/WorkModalContext'
import type { WorkModalItem } from '../components/WorkModalContext'
import { useKind } from '../hooks/useKind'
import { api } from '../lib/api'
import { STATUS_LABELS, containsKana, containsKanji, formatScore, kanaToRomaji, translateGenre } from '../lib/util'
import type { AniListItem, LibraryRecord } from '../types'

// 同じ作品（anilistId）を除いた上で、AniListの人気値（popularity）降順にまとめる。
// ひらがな/カタカナ入力時、元のかな検索とローマ字変換検索の両方を実行して結果をマージするために使う。
function mergeSearchResultsByPopularity(a: AniListItem[], b: AniListItem[]): AniListItem[] {
  const byId = new Map<number, AniListItem>()
  for (const item of [...a, ...b]) {
    if (!byId.has(item.anilistId)) byId.set(item.anilistId, item)
  }
  return Array.from(byId.values()).sort((x, y) => (y.popularity ?? 0) - (x.popularity ?? 0))
}

type Idle = { kind: 'idle'; tsumi: LibraryRecord[]; trend: AniListItem[]; libraryByAniListId: Map<number, LibraryRecord> }
type Results = { kind: 'results'; items: AniListItem[]; libraryByAniListId: Map<number, LibraryRecord> }
type Loading = { kind: 'loading' }
type ErrorState = { kind: 'error'; message: string }
type ViewState = Idle | Results | Loading | ErrorState

export function SearchPage() {
  const [kind, setKind] = useKind()
  const [query, setQuery] = useState('')
  const [view, setView] = useState<ViewState>({ kind: 'loading' })
  const openWorkModal = useWorkModal()

  const isComposingRef = useRef(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const requestSeqRef = useRef(0)

  const runSearch = (q: string, currentKind: typeof kind) => {
    const trimmed = q.trim()
    const seq = ++requestSeqRef.current

    if (trimmed === '') {
      setView({ kind: 'loading' })
      Promise.all([api.listRecords({ type: currentKind }), api.searchAniList({ type: currentKind, q: '' })])
        .then(([libraryRecords, trendResults]) => {
          if (seq !== requestSeqRef.current) return
          setView({
            kind: 'idle',
            tsumi: libraryRecords.filter((r) => r.status === 'want'),
            trend: trendResults.slice(0, 5),
            libraryByAniListId: new Map(libraryRecords.map((r) => [r.anilistId, r])),
          })
        })
        .catch((err) => {
          if (seq !== requestSeqRef.current) return
          setView({ kind: 'error', message: err instanceof Error ? err.message : String(err) })
        })
      return
    }

    setView({ kind: 'loading' })
    const romaji = containsKana(trimmed) && !containsKanji(trimmed) ? kanaToRomaji(trimmed) : null
    const shouldMergeRomaji = !!romaji && romaji !== trimmed

    Promise.all([
      api.searchAniList({ type: currentKind, q }),
      shouldMergeRomaji ? api.searchAniList({ type: currentKind, q: romaji! }) : Promise.resolve([]),
      api.listRecords({ type: currentKind }),
    ])
      .then(([searchResults, romajiResults, libraryRecords]) => {
        if (seq !== requestSeqRef.current) return
        const items = shouldMergeRomaji ? mergeSearchResultsByPopularity(searchResults, romajiResults) : searchResults
        setView({ kind: 'results', items, libraryByAniListId: new Map(libraryRecords.map((r) => [r.anilistId, r])) })
      })
      .catch((err) => {
        if (seq !== requestSeqRef.current) return
        setView({ kind: 'error', message: err instanceof Error ? err.message : String(err) })
      })
  }

  const scheduleSearch = (q: string, currentKind: typeof kind, delayMs = 500) => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(q, currentKind), delayMs)
  }

  // マウント時・kind切り替え時は即時検索する。
  useEffect(() => {
    runSearch(query, kind)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind])

  const reload = () => runSearch(query, kind)

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-4)', marginBottom: 'var(--space-6)', flexWrap: 'wrap' }}>
        <h2 style={{ margin: 0 }}>検索・追加</h2>
        <KindToggle kind={kind} onChange={setKind} />
      </div>

      <div className="field" style={{ marginBottom: 'var(--space-6)' }}>
        <input
          className="input"
          type="text"
          placeholder={kind === 'anime' ? 'アニメを検索' : '漫画を検索'}
          value={query}
          onChange={(e) => {
            const next = e.target.value
            setQuery(next)
            if (!isComposingRef.current) scheduleSearch(next, kind)
          }}
          onCompositionStart={() => {
            isComposingRef.current = true
          }}
          onCompositionEnd={(e) => {
            isComposingRef.current = false
            setQuery(e.currentTarget.value)
            scheduleSearch(e.currentTarget.value, kind)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !isComposingRef.current) {
              if (debounceRef.current) clearTimeout(debounceRef.current)
              runSearch(query, kind)
            }
          }}
        />
      </div>

      {view.kind === 'loading' && (
        <p className="text-muted" style={{ fontSize: 13 }}>
          読み込み中…
        </p>
      )}
      {view.kind === 'error' && <p className="text-muted">検索に失敗しました: {view.message}</p>}
      {view.kind === 'results' && (
        <SearchResults kind={kind} query={query} items={view.items} libraryByAniListId={view.libraryByAniListId} onOpen={(item, record) => openWorkModal({ item, mediaType: kind, record, onChange: reload })} />
      )}
      {view.kind === 'idle' && (
        <IdleState kind={kind} tsumi={view.tsumi} trend={view.trend} libraryByAniListId={view.libraryByAniListId} onOpen={(item, record) => openWorkModal({ item, mediaType: kind, record, onChange: reload })} />
      )}
    </>
  )
}

function SearchResults({
  kind,
  query,
  items,
  libraryByAniListId,
  onOpen,
}: {
  kind: 'anime' | 'manga'
  query: string
  items: AniListItem[]
  libraryByAniListId: Map<number, LibraryRecord>
  onOpen: (item: AniListItem, record: LibraryRecord | null) => void
}) {
  if (items.length === 0) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--space-3)', padding: 'var(--space-8) 0' }}>
        <p className="text-muted" style={{ fontSize: 13, margin: 0 }}>
          「{query}」に一致する作品が見つかりません。
        </p>
      </div>
    )
  }
  return (
    <>
      <p className="text-muted" style={{ fontSize: 12, margin: '0 0 var(--space-3)' }}>
        {items.length}件の作品
      </p>
      <div className="poster-grid">
        {items.map((item) => {
          const record = libraryByAniListId.get(item.anilistId) || null
          return (
            <PosterCard
              key={item.anilistId}
              item={item}
              badgeLabel={record ? `${STATUS_LABELS[kind][record.status]} ✓` : null}
              caption={(item.genres || []).slice(0, 2).map(translateGenre).join('・')}
              onClick={() => onOpen(item, record)}
            />
          )
        })}
      </div>
    </>
  )
}

function IdleState({
  kind,
  tsumi,
  trend,
  libraryByAniListId,
  onOpen,
}: {
  kind: 'anime' | 'manga'
  tsumi: LibraryRecord[]
  trend: AniListItem[]
  libraryByAniListId: Map<number, LibraryRecord>
  onOpen: (item: WorkModalItem, record: LibraryRecord | null) => void
}) {
  const trendTitle = kind === 'anime' ? '今季人気 TOP5' : '人気の漫画 TOP5'
  return (
    <>
      <div className="shelf">
        <h5 className="shelf-h" style={{ marginBottom: 4 }}>
          積み作品
        </h5>
        <p className="text-muted shelf-desc">「見たい・読みたい」に入れたまま手をつけていない作品</p>
        {tsumi.length === 0 ? (
          <p className="text-muted" style={{ fontSize: 13 }}>
            積み作品はありません。
          </p>
        ) : (
          <div className="poster-grid">
            {tsumi.map((r) => (
              <PosterCard key={r.id} item={r} onClick={() => onOpen(r, r)} />
            ))}
          </div>
        )}
      </div>

      <div className="shelf">
        <h5 className="shelf-h">{trendTitle}</h5>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, maxWidth: 640 }}>
          {trend.map((item, i) => {
            const record = libraryByAniListId.get(item.anilistId) || null
            const score = formatScore(item.score)
            return (
              <div
                key={item.anilistId}
                className="card elev-sm"
                style={{ flexDirection: 'row', alignItems: 'center', gap: 'var(--space-3)', padding: '9px var(--space-3)', cursor: 'pointer' }}
                onClick={() => onOpen(item, record)}
              >
                <span style={{ fontFamily: 'var(--font-heading)', fontWeight: 700, fontSize: 16, color: 'var(--color-accent)', width: 18, flex: 'none' }}>
                  {i + 1}
                </span>
                <Thumb item={item} width="32px" height="42px" fontSize={13} />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="card-title" style={{ fontSize: 13, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {item.title}
                  </div>
                  <div className="text-muted" style={{ fontSize: 11, marginTop: 2 }}>
                    {(item.genres || []).slice(0, 2).map(translateGenre).join('・')}
                  </div>
                </div>
                {record && (
                  <span className="tag tag-accent" style={{ flex: 'none' }}>
                    {STATUS_LABELS[kind][record.status]} ✓
                  </span>
                )}
                {score && (
                  <span className="text-muted" style={{ fontSize: 12, flex: 'none' }}>
                    ★{score}
                  </span>
                )}
              </div>
            )
          })}
        </div>
      </div>
    </>
  )
}
