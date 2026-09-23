import { useEffect, useMemo, useState } from 'react'
import { KindToggle } from '../components/KindToggle'
import { PosterCard } from '../components/PosterCard'
import { useWorkModal } from '../components/WorkModalContext'
import { useKind } from '../hooks/useKind'
import { api } from '../lib/api'
import { STATUS_LABELS, translateGenre, unitFor, type Status } from '../lib/util'
import type { LibraryRecord } from '../types'

const STATUS_ORDER: Status[] = ['done', 'active', 'want']

export function LibraryPage() {
  const [kind, setKind] = useKind()
  const [status, setStatus] = useState<Status>('active')
  const [activeGenres, setActiveGenres] = useState<string[]>([])
  const [allRecords, setAllRecords] = useState<LibraryRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadSeq, setReloadSeq] = useState(0)
  const openWorkModal = useWorkModal()

  useEffect(() => {
    setActiveGenres([])
  }, [kind])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    api
      .listRecords({ type: kind })
      .then((records) => {
        if (cancelled) return
        setAllRecords(records)
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
  }, [kind, reloadSeq])

  const reload = () => setReloadSeq((n) => n + 1)
  const labels = STATUS_LABELS[kind]

  const counts = useMemo(() => {
    const c: Record<Status, number> = { done: 0, active: 0, want: 0 }
    for (const r of allRecords) c[r.status]++
    return c
  }, [allRecords])

  const genreDist = useMemo(() => {
    const c = new Map<string, number>()
    for (const r of allRecords) {
      for (const g of r.genres) c.set(g, (c.get(g) || 0) + 1)
    }
    const total = Array.from(c.values()).reduce((a, b) => a + b, 0)
    const top = Array.from(c.entries()).sort((a, b) => b[1] - a[1]).slice(0, 3)
    return top.map(([genre, count]) => ({ genre, pct: Math.round((count / total) * 100) }))
  }, [allRecords])

  const statusPool = useMemo(() => allRecords.filter((r) => r.status === status), [allRecords, status])
  const genreChips = useMemo(() => Array.from(new Set(statusPool.flatMap((r) => r.genres))), [statusPool])

  const results = useMemo(() => {
    if (activeGenres.length === 0) return statusPool
    return statusPool.filter((r) => r.genres.some((g) => activeGenres.includes(g)))
  }, [statusPool, activeGenres])

  const toggleGenre = (g: string) => {
    setActiveGenres((prev) => (prev.includes(g) ? prev.filter((x) => x !== g) : [...prev, g]))
  }

  const unit = unitFor()

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-4)', marginBottom: 'var(--space-6)', flexWrap: 'wrap' }}>
        <h2 style={{ margin: 0 }}>マイライブラリ</h2>
        <KindToggle kind={kind} onChange={setKind} />
      </div>

      <div style={{ display: 'flex', gap: 'var(--space-3)', marginBottom: 'var(--space-6)', flexWrap: 'wrap' }}>
        {STATUS_ORDER.map((key) => (
          <div key={key} className="card elev-sm stat-card">
            <div className="card-meta">{labels[key]}</div>
            <div style={{ fontFamily: 'var(--font-heading)', fontSize: 24, fontWeight: 600 }}>{counts[key]}本</div>
          </div>
        ))}
        <div className="card elev-sm genre-dist-card">
          <div className="card-meta">ジャンル分布</div>
          {genreDist.length === 0 ? (
            <p className="text-muted" style={{ fontSize: 12, margin: 0 }}>
              まだ記録がありません。
            </p>
          ) : (
            genreDist.map(({ genre, pct }) => (
              <div key={genre}>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, marginBottom: 3 }}>
                  <span>{translateGenre(genre)}</span>
                  <span className="text-muted">{pct}%</span>
                </div>
                <div className="genre-dist-bar">
                  <div className="genre-dist-bar-fill" style={{ width: `${pct}%` }} />
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      <div className="seg" style={{ width: 'fit-content' }}>
        {STATUS_ORDER.map((key) => (
          <label key={key} className={`seg-opt${status === key ? ' checked' : ''}`}>
            <input type="radio" name="libstatus" checked={status === key} onChange={() => setStatus(key)} />
            <span className="seg-label">{labels[key]}</span>
          </label>
        ))}
      </div>

      {genreChips.length > 0 && (
        <div style={{ marginBottom: 'var(--space-6)' }}>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {genreChips.map((g) => (
              <span key={g} className={`tag ${activeGenres.includes(g) ? 'tag-accent' : 'tag-outline'}`} onClick={() => toggleGenre(g)}>
                {translateGenre(g)}
              </span>
            ))}
          </div>
        </div>
      )}

      {loading ? (
        <p className="text-muted">読み込み中…</p>
      ) : error ? (
        <p className="text-muted">読み込みに失敗しました: {error}</p>
      ) : results.length === 0 ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          該当する作品がありません。
        </p>
      ) : (
        <div className="poster-grid">
          {results.map((r) => {
            const pct = r.status === 'active' && r.total ? Math.min(100, Math.round((r.progress / r.total) * 100)) : null
            const caption =
              r.status === 'active'
                ? r.total
                  ? `${r.progress} / ${r.total}${unit}`
                  : `${r.progress}${unit}まで`
                : (r.genres || []).slice(0, 2).map(translateGenre).join('・')
            return (
              <PosterCard
                key={r.id}
                item={r}
                pct={pct}
                caption={caption}
                onClick={() => openWorkModal({ item: r, mediaType: kind, record: r, onChange: reload })}
              />
            )
          })}
        </div>
      )}
    </>
  )
}
