import { useEffect, useState } from 'react'
import { PosterCard } from '../components/PosterCard'
import { useWorkModal } from '../components/WorkModalContext'
import { useAuth } from '../context/AuthContext'
import { api } from '../lib/api'
import { STATUS_LABELS } from '../lib/util'
import type { LibraryRecord, NewReleaseItem, NewReleasesResult, NewSeriesItem } from '../types'

// 「新着」タブ。ホームの「掲載誌で探す」の代わりに足した（2026-10-25、BACKLOG 11）。
// 新刊は楽天ブックス（11-b）、新連載は AniList（11-a）から取る。

type Tab = 'releases' | 'series'

const WEEKDAYS = ['日', '月', '火', '水', '木', '金', '土']

// "2026-10-23" を「10月23日」にする。年は直近の作品しか出ないので省く
function formatMonthDay(date: string): string {
  const [, month, day] = date.split('-').map(Number)
  return `${month}月${day}日`
}

// "2026-10-23" を「10/23（金）」にする。曜日はタイムゾーンに左右されないよう UTC で数える
function formatDateChip(date: string): string {
  const [year, month, day] = date.split('-').map(Number)
  const weekday = WEEKDAYS[new Date(Date.UTC(year, month - 1, day)).getUTCDay()]
  return `${month}/${day}（${weekday}）`
}

function formatFetchedAt(fetchedAt: string): string {
  const d = new Date(fetchedAt)
  return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours()}:${String(d.getMinutes()).padStart(2, '0')}`
}

export function NewArrivalsPage() {
  const [tab, setTab] = useState<Tab>('releases')

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-4)', marginBottom: 'var(--space-6)', flexWrap: 'wrap' }}>
        <h2 style={{ margin: 0 }}>新着</h2>
        <div className="seg" style={{ flex: 'none', marginBottom: 0 }}>
          {(
            [
              ['releases', '新刊'],
              ['series', '新連載'],
            ] as const
          ).map(([t, label]) => (
            <button key={t} type="button" className={`seg-opt${tab === t ? ' checked' : ''}`} onClick={() => setTab(t)}>
              {label}
            </button>
          ))}
        </div>
      </div>
      {/* 両方を描画したまま表示だけ切り替える。切り替えのたびに取り直さないため */}
      <div hidden={tab !== 'releases'}>
        <NewReleasesSection />
      </div>
      <div hidden={tab !== 'series'}>
        <NewSeriesSection />
      </div>
    </>
  )
}

function NewReleasesSection() {
  const [result, setResult] = useState<NewReleasesResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [selectedDate, setSelectedDate] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    api
      .newReleases()
      .then((r) => {
        if (!cancelled) setResult(r)
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (error) return <p className="text-muted">新刊の取得に失敗しました: {error}</p>
  if (!result) return <p className="text-muted">読み込み中…</p>
  if (!result.configured) {
    return <p className="text-muted">新刊は準備中です（楽天ブックスの設定がまだありません）。</p>
  }

  // 発売日ごとにまとめる。サーバーは発売日の新しい順に返すので、出てきた順がそのまま日付の新しい順
  const byDate = new Map<string, NewReleaseItem[]>()
  for (const item of result.items) {
    const list = byDate.get(item.salesDate)
    if (list) list.push(item)
    else byDate.set(item.salesDate, [item])
  }
  const dates = Array.from(byDate.keys())
  // 1 週間で 200〜300 冊あり、表紙は背景画像なので遅延読み込みが効かない。全部を一度に描くと
  // 表紙をまとめて読み込むので、発売日で絞って出す。最初は最新の発売日
  const active = selectedDate && byDate.has(selectedDate) ? selectedDate : dates[0]
  const items = active ? (byDate.get(active) ?? []) : []

  return (
    <section>
      <p className="text-muted shelf-desc">
        {result.from && result.to ? `${formatMonthDay(result.from)}〜${formatMonthDay(result.to)}に発売されたコミック` : '最近発売されたコミック'}
        {result.fetchedAt && `（楽天ブックス、${formatFetchedAt(result.fetchedAt)} 時点）`}
      </p>
      {result.truncated && (
        <p className="text-muted" style={{ fontSize: 12 }}>
          件数が多いため、一部だけを表示しています。
        </p>
      )}
      {dates.length === 0 ? (
        <p className="text-muted">この期間の新刊は見つかりませんでした。</p>
      ) : (
        <>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 'var(--space-4)' }}>
            {dates.map((date) => (
              <button
                key={date}
                type="button"
                className={`tag ${date === active ? 'tag-accent' : 'tag-outline'}`}
                onClick={() => setSelectedDate(date)}
              >
                {formatDateChip(date)} {byDate.get(date)?.length}
              </button>
            ))}
          </div>
          <div className="poster-grid">
            {items.map((item) => (
              <PosterCard
                key={item.isbn || item.url}
                item={{ title: item.title, coverImageUrl: item.imageUrl }}
                caption={[item.author, item.salesDateApproximate ? '発売日は予定' : null].filter(Boolean).join('・')}
                // 楽天の本は AniList の作品と結び付けていないので（BACKLOG 11-b）、作品モーダルではなく楽天の商品ページを開く
                onClick={() => window.open(item.url, '_blank', 'noopener,noreferrer')}
              />
            ))}
          </div>
        </>
      )}
    </section>
  )
}

function NewSeriesSection() {
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
    <section>
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
                caption={`${formatMonthDay(item.startedOn)}開始`}
                onClick={() => openWorkModal({ item, mediaType: 'manga', record, onChange: reload })}
              />
            )
          })}
        </div>
      )}
    </section>
  )
}
