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

export function SeasonPage() {
  const { user } = useAuth()
  const openWorkModal = useWorkModal()
  const [{ season, year }, setSeasonYear] = useState(currentSeasonClient)
  const [results, setResults] = useState<AniListItem[]>([])
  const [library, setLibrary] = useState<LibraryRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadSeq, setReloadSeq] = useState(0)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)

    Promise.all([api.seasonAnime({ season, year }), user ? api.listRecords({ type: 'anime' }) : Promise.resolve([])])
      .then(([r, l]) => {
        if (cancelled) return
        setResults(r)
        setLibrary(l)
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
  }, [season, year, user, reloadSeq])

  const reload = () => setReloadSeq((n) => n + 1)
  const libraryByAniListId = new Map(library.map((r) => [r.anilistId, r]))
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
          {results.map((item) => {
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
    </>
  )
}
