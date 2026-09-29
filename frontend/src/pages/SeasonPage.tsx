import { useEffect, useState } from 'react'
import { PosterCard } from '../components/PosterCard'
import { useWorkModal } from '../components/WorkModalContext'
import { useAuth } from '../context/AuthContext'
import { api } from '../lib/api'
import { STATUS_LABELS, formatScore, translateGenre } from '../lib/util'
import type { AniListItem, LibraryRecord } from '../types'

const SEASON_LABELS: Record<string, string> = { WINTER: '冬', SPRING: '春', SUMMER: '夏', FALL: '秋' }
const SEASONS = ['WINTER', 'SPRING', 'SUMMER', 'FALL']

// AniListの慣例に合わせる（12月は翌年のWINTERシーズン扱い）。ホームのシーズン算出（サーバー側）と同じロジック。
function currentSeasonClient(now = new Date()): { season: string; year: number } {
  const month = now.getMonth() + 1
  const year = now.getFullYear()
  if (month === 12) return { season: 'WINTER', year: year + 1 }
  if (month <= 2) return { season: 'WINTER', year }
  if (month <= 5) return { season: 'SPRING', year }
  if (month <= 8) return { season: 'SUMMER', year }
  return { season: 'FALL', year }
}

const YEAR_MIN = 2020

// PAGE_SIZE は1ページに並べる件数。サーバーはシーズンあたり40件（AniListの1リクエスト上限50の内側）を
// まとめて返すため、ページ送りは取得済みの配列を切り出すだけで済み、追加のAPI呼び出しは発生しない。
const PAGE_SIZE = 20

export function SeasonPage() {
  const { user } = useAuth()
  const openWorkModal = useWorkModal()
  const [{ season, year }, setSeasonYear] = useState(currentSeasonClient)
  const [results, setResults] = useState<AniListItem[]>([])
  const [library, setLibrary] = useState<LibraryRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadSeq, setReloadSeq] = useState(0)
  const [page, setPage] = useState(1)

  // シーズン一覧はseason/yearだけに依存する。ログイン状態やライブラリの更新では取り直さない
  // （AniListへの問い合わせはレート制限付きで高コストなため、不要な再取得を避ける）。
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    setPage(1) // シーズンを切り替えたら1ページ目から見せる

    api
      .seasonAnime({ season, year })
      .then((r) => {
        if (cancelled) return
        setResults(r)
        setLoading(false)
      })
      .catch((err) => {
        if (cancelled) return
        setError(err instanceof Error ? err.message : String(err))
        setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [season, year])

  // ライブラリ（登録済みバッジ用）はseason/yearに依存しないので、ログイン状態が変わったときと
  // モーダルでの更新後にだけ取り直す。取得に失敗してもバッジが出ないだけなので一覧は止めない。
  useEffect(() => {
    if (!user) {
      setLibrary([])
      return
    }
    let cancelled = false

    api
      .listRecords({ type: 'anime' })
      .then((l) => {
        if (!cancelled) setLibrary(l)
      })
      .catch(() => {
        if (!cancelled) setLibrary([])
      })

    return () => {
      cancelled = true
    }
  }, [user, reloadSeq])

  const reload = () => setReloadSeq((n) => n + 1)
  const libraryByAniListId = new Map(library.map((r) => [r.anilistId, r]))
  const totalPages = Math.max(1, Math.ceil(results.length / PAGE_SIZE))
  const pageItems = results.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)

  // ページを送ったら一覧の先頭が見えるように戻す（下端のボタンを押した位置のままだと
  // 切り替わった一覧の途中から始まってしまうため）。
  const goToPage = (next: number) => {
    setPage(next)
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
  const currentYear = new Date().getFullYear()
  const years: number[] = []
  for (let y = currentYear; y >= YEAR_MIN; y--) years.push(y)

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-4)', marginBottom: 'var(--space-6)', flexWrap: 'wrap' }}>
        <h2 style={{ margin: 0 }}>シーズン</h2>
        <div style={{ display: 'flex', gap: 8, flex: 'none' }}>
          <select
            className="input"
            style={{ width: 'auto', minHeight: 34, padding: '4px 12px' }}
            value={year}
            onChange={(e) => setSeasonYear({ season, year: Number(e.target.value) })}
          >
            {years.map((y) => (
              <option key={y} value={y}>
                {y}年
              </option>
            ))}
          </select>
          <select
            className="input"
            style={{ width: 'auto', minHeight: 34, padding: '4px 12px' }}
            value={season}
            onChange={(e) => setSeasonYear({ season: e.target.value, year })}
          >
            {SEASONS.map((s) => (
              <option key={s} value={s}>
                {SEASON_LABELS[s]}
              </option>
            ))}
          </select>
        </div>
      </div>

      <p className="text-muted" style={{ margin: '0 0 var(--space-4)' }}>
        {year}年 {SEASON_LABELS[season]}アニメ
      </p>

      {loading ? (
        <p className="text-muted">読み込み中…</p>
      ) : error ? (
        <p className="text-muted">AniListからの取得に失敗しました: {error}</p>
      ) : results.length === 0 ? (
        <p className="text-muted">該当するアニメが見つかりませんでした。</p>
      ) : (
        <div className="poster-grid">
          {pageItems.map((item) => {
            const record = libraryByAniListId.get(item.anilistId) || null
            const score = formatScore(item.score)
            return (
              <PosterCard
                key={item.anilistId}
                item={item}
                badgeLabel={record ? `${STATUS_LABELS.anime[record.status]} ✓` : null}
                caption={[score ? `★${score}` : null, ...(item.genres || []).slice(0, 2).map(translateGenre)].filter(Boolean).join('・')}
                onClick={() => openWorkModal({ item, mediaType: 'anime', record, onChange: reload })}
              />
            )
          })}
        </div>
      )}

      {!loading && !error && totalPages > 1 && (
        <nav
          aria-label="ページ送り"
          style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 'var(--space-4)', marginTop: 'var(--space-6)' }}
        >
          <button type="button" className="btn btn-secondary" disabled={page === 1} onClick={() => goToPage(page - 1)}>
            ‹ 前へ
          </button>
          <span className="text-muted" style={{ fontSize: 13 }} aria-live="polite">
            {page} / {totalPages}
          </span>
          <button type="button" className="btn btn-secondary" disabled={page === totalPages} onClick={() => goToPage(page + 1)}>
            次へ ›
          </button>
        </nav>
      )}
    </>
  )
}
