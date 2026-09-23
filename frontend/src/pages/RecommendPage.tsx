import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { KindToggle } from '../components/KindToggle'
import { PosterCard } from '../components/PosterCard'
import { useWorkModal } from '../components/WorkModalContext'
import type { WorkModalItem } from '../components/WorkModalContext'
import { useAuth } from '../context/AuthContext'
import { useKind } from '../hooks/useKind'
import { api } from '../lib/api'
import { formatScore, translateGenre } from '../lib/util'
import type { AniListItem, LibraryRecord } from '../types'

interface Shelf {
  title: string
  reason: string
  items: WorkModalItem[]
  isLibraryItems?: boolean
}

function topGenres(records: LibraryRecord[]): string[] {
  const counts = new Map<string, number>()
  for (const r of records) {
    if (r.status !== 'done' && r.status !== 'active') continue
    for (const g of r.genres) counts.set(g, (counts.get(g) || 0) + 1)
  }
  return Array.from(counts.entries())
    .sort((a, b) => b[1] - a[1])
    .slice(0, 3)
    .map(([genre]) => genre)
}

export function RecommendPage() {
  const { user } = useAuth()
  const [kind, setKind] = useKind()
  const [loading, setLoading] = useState(true)
  const [shelves, setShelves] = useState<Shelf[]>([])
  const [error, setError] = useState<string | null>(null)
  const openWorkModal = useWorkModal()
  const [reloadSeq, setReloadSeq] = useState(0)

  useEffect(() => {
    if (!user) return
    let cancelled = false
    setLoading(true)
    setError(null)

    ;(async () => {
      let library: LibraryRecord[] = []
      try {
        library = await api.listRecords({ type: kind })
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : String(err))
        setLoading(false)
        return
      }

      const genres = topGenres(library)
      const registeredIds = new Set(library.map((r) => r.anilistId))
      const tsumi = library.filter((r) => r.status === 'want')

      let genreBased: AniListItem[] = []
      if (genres.length > 0) {
        try {
          genreBased = (await api.recommendations({ type: kind, genres })).filter((item) => !registeredIds.has(item.anilistId))
        } catch {
          genreBased = []
        }
      }
      if (cancelled) return

      const highScore = genreBased.filter((item) => item.score != null && item.score >= 80).sort((a, b) => (b.score ?? 0) - (a.score ?? 0))

      const next: Shelf[] = []
      if (genreBased.length > 0) {
        next.push({
          title: 'いま記録している作品と近い雰囲気',
          reason: `記録済みのジャンル（${genres.map(translateGenre).join('・')}）から`,
          items: genreBased.slice(0, 10),
        })
      }
      if (highScore.length > 0) {
        next.push({ title: '評価が高い定番', reason: '★8.0以上・未記録の作品', items: highScore.slice(0, 10) })
      }
      if (tsumi.length > 0) {
        next.push({ title: '積み作品から', reason: '「見たい・読みたい」に入れたまま止まっている作品', items: tsumi, isLibraryItems: true })
      }

      setShelves(next)
      setLoading(false)
    })()

    return () => {
      cancelled = true
    }
  }, [user, kind, reloadSeq])

  const reload = () => setReloadSeq((n) => n + 1)

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-4)', marginBottom: 'var(--space-6)', flexWrap: 'wrap' }}>
        <h2 style={{ margin: 0 }}>おすすめ</h2>
        {user && <KindToggle kind={kind} onChange={setKind} />}
      </div>

      {!user ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          ログインするとあなたの視聴傾向に合わせたおすすめが表示されます。 <Link to="/login">ログイン</Link>
        </p>
      ) : loading ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          読み込み中…
        </p>
      ) : error ? (
        <p className="text-muted">読み込みに失敗しました: {error}</p>
      ) : shelves.length === 0 ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          記録がまだありません。作品を検索してライブラリに追加すると、あなたの好みに合わせたおすすめが表示されます。
        </p>
      ) : (
        shelves.map((shelf) => (
          <div className="shelf" key={shelf.title}>
            <h5 className="shelf-h" style={{ marginBottom: 4 }}>
              {shelf.title}
            </h5>
            <p className="text-muted shelf-desc">{shelf.reason}</p>
            <div className="poster-row">
              {shelf.items.map((item) => {
                const record = shelf.isLibraryItems ? (item as unknown as LibraryRecord) : null
                const score = formatScore(item.score)
                return (
                  <PosterCard
                    key={item.anilistId}
                    item={item}
                    caption={score ? `★${score}` : null}
                    onClick={() => openWorkModal({ item, mediaType: kind, record, onChange: reload })}
                  />
                )
              })}
            </div>
          </div>
        ))
      )}
    </>
  )
}
