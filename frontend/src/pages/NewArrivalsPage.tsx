import { useEffect, useState } from 'react'
import { PosterCard } from '../components/PosterCard'
import { useWorkModal } from '../components/WorkModalContext'
import { useAuth } from '../context/AuthContext'
import { api } from '../lib/api'
import { STATUS_LABELS } from '../lib/util'
import type { LibraryRecord, NewSeriesItem } from '../types'

// 「新着」タブ。ホームの「掲載誌で探す」の代わりに足した（2026-10-25、BACKLOG 11）。
// 新連載は AniList から取る。新刊（楽天ブックス）は別の PR でサーバー側を作っていて、
// そちらがマージされたらこの画面に節を足す。

// "2026-10-23" を「10月23日」にする。年は今年の作品しか出ないので省く
function formatStartedOn(startedOn: string): string {
  const [, month, day] = startedOn.split('-').map(Number)
  return `${month}月${day}日`
}

export function NewArrivalsPage() {
  const { user } = useAuth()
  const openWorkModal = useWorkModal()
  const [series, setSeries] = useState<NewSeriesItem[]>([])
  const [library, setLibrary] = useState<LibraryRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadSeq, setReloadSeq] = useState(0)

  useEffect(() => {
    let cancelled = false
    api
      .newSeries()
      .then((r) => {
        if (cancelled) return
        setSeries(r)
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
  }, [])

  // 登録済みバッジ用。失敗してもバッジが出ないだけなので一覧は止めない（シーズン画面と同じ）
  useEffect(() => {
    if (!user) return
    let cancelled = false
    api
      .listRecords({ type: 'manga' })
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

  return (
    <>
      <h2 style={{ margin: '0 0 var(--space-6)' }}>新着</h2>

      <section>
        <h5 className="shelf-h">新連載</h5>
        <p className="text-muted shelf-desc">この 3 か月に始まった漫画の連載（人気順）</p>
        {loading ? (
          <p className="text-muted">読み込み中…</p>
        ) : error ? (
          <p className="text-muted">AniListからの取得に失敗しました: {error}</p>
        ) : series.length === 0 ? (
          <p className="text-muted">新しく始まった連載が見つかりませんでした。</p>
        ) : (
          <div className="poster-grid">
            {series.map((item) => {
              const record = libraryByAniListId.get(item.anilistId) || null
              return (
                <PosterCard
                  key={item.anilistId}
                  item={item}
                  badgeLabel={record ? `${STATUS_LABELS.manga[record.status]} ✓` : null}
                  caption={`${formatStartedOn(item.startedOn)}開始`}
                  onClick={() => openWorkModal({ item, mediaType: 'manga', record, onChange: reload })}
                />
              )
            })}
          </div>
        )}
      </section>
    </>
  )
}
