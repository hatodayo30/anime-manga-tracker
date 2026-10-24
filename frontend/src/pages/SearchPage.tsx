import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { KindToggle } from '../components/KindToggle'
import { PosterCard } from '../components/PosterCard'
import { Thumb } from '../components/Thumb'
import { useWorkModal } from '../components/WorkModalContext'
import { useAuth } from '../context/AuthContext'
import type { WorkModalItem } from '../components/WorkModalContext'
import { useKind } from '../hooks/useKind'
import { api } from '../lib/api'
import { STATUS_LABELS, activateOnKey, containsKana, containsKanji, formatScore, kanaToRomaji, translateGenre } from '../lib/util'
import type { MediaKind } from '../lib/util'
import type { AniListItem, LibraryRecord, NaturalSearchResult } from '../types'

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
// mediaType は結果の種別。AI で探したとき、文で種別を指定していれば画面で選んでいる種別と違うことがある。
// ai は「AI で探す」の結果のときだけ入り、AI が作った条件と従来の検索に落ちた理由を出すのに使う。
type Results = {
  kind: 'results'
  items: AniListItem[]
  mediaType: MediaKind
  libraryByAniListId: Map<number, LibraryRecord>
  ai?: Pick<NaturalSearchResult, 'conditions' | 'fallback'>
}
type Loading = { kind: 'loading' }
type ErrorState = { kind: 'error'; message: string }
type ViewState = Idle | Results | Loading | ErrorState

export function SearchPage() {
  const [kind, setKind] = useKind()
  const [query, setQuery] = useState('')
  const [view, setView] = useState<ViewState>({ kind: 'loading' })
  const openWorkModal = useWorkModal()
  const { user } = useAuth()
  // AI で探す（BACKLOG 9-b）。オンの間は入力のたびに検索せず、Enter で確定したときだけ AI を呼ぶ。
  // 1 回ごとに請求が発生するので、打っている途中の文で何度も呼ばないため。
  const [aiMode, setAiMode] = useState(false)
  const [loginHint, setLoginHint] = useState(false)

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

    if (aiMode) {
      runNaturalSearch(trimmed, currentKind, seq)
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
        setView({ kind: 'results', items, mediaType: currentKind, libraryByAniListId: new Map(libraryRecords.map((r) => [r.anilistId, r])) })
      })
      .catch((err) => {
        if (seq !== requestSeqRef.current) return
        setView({ kind: 'error', message: err instanceof Error ? err.message : String(err) })
      })
  }

  // AI の失敗はサーバー側で従来の検索に落ちて 200 が返るので、ここでエラーになるのは未ログイン（401）か
  // 従来の検索まで失敗したときだけ。ライブラリは結果の種別が決まってから引く（文で種別が変わりうるため）。
  const runNaturalSearch = (q: string, currentKind: MediaKind, seq: number) => {
    setView({ kind: 'loading' })
    api
      .naturalSearch({ type: currentKind, q })
      .then(async (result) => {
        const libraryRecords = await api.listRecords({ type: result.mediaType })
        if (seq !== requestSeqRef.current) return
        setView({
          kind: 'results',
          items: result.items,
          mediaType: result.mediaType,
          libraryByAniListId: new Map(libraryRecords.map((r) => [r.anilistId, r])),
          ai: { conditions: result.conditions, fallback: result.fallback },
        })
      })
      .catch((err) => {
        if (seq !== requestSeqRef.current) return
        const message = err instanceof Error ? err.message : String(err)
        if (message === 'login required') {
          // セッション切れ。従来の検索に戻して案内を出す
          setAiMode(false)
          setLoginHint(true)
        }
        setView({ kind: 'error', message })
      })
  }

  const toggleAiMode = () => {
    if (!user) {
      setLoginHint(true)
      return
    }
    const next = !aiMode
    setAiMode(next)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    // オンにしただけでは AI を呼ばない（Enter で確定してから）。オフに戻したら従来の検索で引き直す
    if (!next) {
      requestSeqRef.current++
      scheduleSearch(query, kind, 0)
    }
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

      <div className="field" style={{ marginBottom: (loginHint && !user) || aiMode ? 'var(--space-2)' : 'var(--space-6)', display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
        <input
          className="input"
          type="text"
          style={{ flex: 1, minWidth: 0 }}
          placeholder={aiMode ? (kind === 'anime' ? '例: 泣ける部活もののアニメ' : '例: 異世界で料理する漫画') : kind === 'anime' ? 'アニメを検索' : '漫画を検索'}
          value={query}
          onChange={(e) => {
            const next = e.target.value
            setQuery(next)
            if (!isComposingRef.current && !aiMode) scheduleSearch(next, kind)
          }}
          onCompositionStart={() => {
            isComposingRef.current = true
          }}
          onCompositionEnd={(e) => {
            isComposingRef.current = false
            setQuery(e.currentTarget.value)
            if (!aiMode) scheduleSearch(e.currentTarget.value, kind)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !isComposingRef.current) {
              if (debounceRef.current) clearTimeout(debounceRef.current)
              runSearch(query, kind)
            }
          }}
        />
        <button type="button" className={`btn ${aiMode ? 'btn-primary' : 'btn-secondary'}`} style={{ flex: 'none' }} aria-pressed={aiMode} onClick={toggleAiMode}>
          AI で探す
        </button>
      </div>
      {loginHint && !user && (
        <p className="text-muted" style={{ fontSize: 12, margin: '0 0 var(--space-6)' }}>
          AI で探すは<Link to="/login?next=%2Fsearch">ログイン</Link>すると使えます。作品名での検索はこのまま使えます。
        </p>
      )}
      {aiMode && (
        <p className="text-muted" style={{ fontSize: 12, margin: '0 0 var(--space-6)' }}>
          探したい作品を文章で書いて Enter で検索します。表示される作品はすべて AniList の検索結果です。
        </p>
      )}

      {view.kind === 'loading' && (
        <p className="text-muted" style={{ fontSize: 13 }}>
          読み込み中…
        </p>
      )}
      {view.kind === 'error' && <p className="text-muted">検索に失敗しました: {view.message}</p>}
      {view.kind === 'results' && (
        <>
          {view.ai && <AISearchSummary mediaType={view.mediaType} ai={view.ai} />}
          <SearchResults
            kind={view.mediaType}
            query={query}
            items={view.items}
            libraryByAniListId={view.libraryByAniListId}
            onOpen={(item, record) => openWorkModal({ item, mediaType: view.mediaType, record, onChange: reload })}
          />
        </>
      )}
      {view.kind === 'idle' && (
        <IdleState kind={kind} tsumi={view.tsumi} trend={view.trend} libraryByAniListId={view.libraryByAniListId} onOpen={(item, record) => openWorkModal({ item, mediaType: kind, record, onChange: reload })} />
      )}
    </>
  )
}

const FALLBACK_MESSAGES: Record<NonNullable<NaturalSearchResult['fallback']>, string> = {
  unavailable: 'AI で探すは現在使えないため、入力をそのままキーワードにして検索しました。',
  failed: 'AI で条件を作れなかったため、入力をそのままキーワードにして検索しました。',
  no_conditions: '文から検索条件を読み取れなかったため、入力をそのままキーワードにして検索しました。',
}

// AI が作った条件を出す。結果が思ったものと違うとき、どう解釈されたかが分かれば文を直せる。
// タグは AniList の英語名のまま出す（ジャンルと違って和訳の表を持っていない）。
function AISearchSummary({ mediaType, ai }: { mediaType: MediaKind; ai: Pick<NaturalSearchResult, 'conditions' | 'fallback'> }) {
  if (ai.fallback || !ai.conditions) {
    return (
      <p className="text-muted" style={{ fontSize: 12, margin: '0 0 var(--space-3)' }}>
        {FALLBACK_MESSAGES[ai.fallback ?? 'failed']}
      </p>
    )
  }
  const c = ai.conditions
  const chips: { label: string; excluded?: boolean }[] = [
    ...(c.mediaType ? [{ label: mediaType === 'anime' ? 'アニメ' : '漫画' }] : []),
    ...c.genres.map((g) => ({ label: translateGenre(g) })),
    ...c.tags.map((t) => ({ label: t })),
    ...(c.query ? [{ label: `「${c.query}」` }] : []),
    ...c.excludedGenres.map((g) => ({ label: translateGenre(g), excluded: true })),
    ...c.excludedTags.map((t) => ({ label: t, excluded: true })),
  ]
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 6, margin: '0 0 var(--space-3)' }}>
      <span className="text-muted" style={{ fontSize: 12 }}>
        AI の検索条件:
      </span>
      {chips.map((chip, i) => (
        <span key={i} className={`tag ${chip.excluded ? 'tag-neutral' : 'tag-accent'}`}>
          {chip.excluded ? `除外: ${chip.label}` : chip.label}
        </span>
      ))}
    </div>
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
                role="button"
                tabIndex={0}
                onKeyDown={activateOnKey(() => onOpen(item, record))}
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
