import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { PosterCard } from '../components/PosterCard'
import { Thumb } from '../components/Thumb'
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

// 取得中（サーバーの起動直後など）に取り直すまでの間隔。全ジャンルの取得は 1 分ほどかかる
const LOADING_RETRY_MS = 15_000

// seriesTitleOf は楽天の書名から巻数や版の表記を落とし、AniList で作品を探すための作品名にする。
// 「呪術廻戦 30」「ダンジョン飯（14）」「〇〇 12巻 特装版」のような書名を想定している。落としきれなくても、
// 検索画面で作品名として直せばよいので、ここでは目立つ形だけを扱う
function seriesTitleOf(title: string): string {
  return title
    .replace(/[【［[][^】］\]]*[】］\]]/g, ' ') // 【特装版】［限定版］
    .replace(/[（(][^）)]*[）)]/g, ' ') // （14）(上)
    .replace(/(特装版|限定版|通常版|特別版|新装版)/g, ' ')
    .trim()
    // 末尾の巻数。空白・句読点の後ろか「巻」付きのときだけ落とす（「天下」の「下」のような題名の一部を落とさない）。
    // 句読点の後ろは「〜されています。6」のような長い題名の実例（楽天、2026-10-25）
    .replace(/(\s+(第?\d+巻?|[上中下]巻?)|第?\d+巻|(?<=[。！？!?])\d+)$/u, '')
    .replace(/\s+/g, ' ')
    .trim()
}

function NewReleasesSection() {
  const [result, setResult] = useState<NewReleasesResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [selectedDate, setSelectedDate] = useState<string | null>(null)
  const [selectedGenre, setSelectedGenre] = useState<string | null>(null)
  const [openBook, setOpenBook] = useState<NewReleaseItem | null>(null)
  const [retrySeq, setRetrySeq] = useState(0)

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
  }, [retrySeq])

  // サーバーがまだ取り終えていなければ、しばらくしてから取り直す
  useEffect(() => {
    if (!result?.loading) return
    const timer = setTimeout(() => setRetrySeq((n) => n + 1), LOADING_RETRY_MS)
    return () => clearTimeout(timer)
  }, [result])

  if (error) return <p className="text-muted">新刊の取得に失敗しました: {error}</p>
  if (!result) return <p className="text-muted">読み込み中…</p>
  if (!result.configured) {
    return <p className="text-muted">新刊は準備中です（楽天ブックスの設定がまだありません）。</p>
  }
  if (result.loading) {
    return (
      <section>
        <p className="text-muted">新刊の一覧を取得しています。1 分ほどで表示されます…</p>
        <RakutenCredit />
      </section>
    )
  }

  const genres = Array.from(new Set(result.items.map((item) => item.genre)))
  const activeGenre = selectedGenre && genres.includes(selectedGenre) ? selectedGenre : null
  const inGenre = activeGenre ? result.items.filter((item) => item.genre === activeGenre) : result.items

  // 発売日ごとにまとめる。サーバーは発売日の新しい順に返すので、出てきた順がそのまま日付の新しい順
  const byDate = new Map<string, NewReleaseItem[]>()
  for (const item of inGenre) {
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
        {result.fetchedAt && `（${formatFetchedAt(result.fetchedAt)} 時点）`}
      </p>
      {result.truncated && (
        <p className="text-muted" style={{ fontSize: 12 }}>
          件数が多いため、一部だけを表示しています。
        </p>
      )}
      {genres.length > 1 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 'var(--space-3)' }}>
          {[null, ...genres].map((g) => (
            <button
              key={g ?? 'all'}
              type="button"
              className={`tag ${g === activeGenre ? 'tag-accent' : 'tag-outline'}`}
              onClick={() => setSelectedGenre(g)}
            >
              {g ?? 'すべて'}
            </button>
          ))}
        </div>
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
                onClick={() => setOpenBook(item)}
              />
            ))}
          </div>
        </>
      )}
      <RakutenCredit />
      {openBook && <BookDialog book={openBook} onClose={() => setOpenBook(null)} />}
    </section>
  )
}

// 楽天ウェブサービスのクレジット表示（必須）。https://webservice.rakuten.co.jp/guide/credit の
// テキストの形をそのまま使う（ガイドは HTML を改変しないよう求めている）
function RakutenCredit() {
  return (
    <p style={{ fontSize: 11, marginTop: 'var(--space-6)' }}>
      {/* eslint-disable-next-line react/jsx-no-target-blank -- 楽天のガイドの HTML をそのまま使う */}
      <a href="https://developers.rakuten.com/" target="_blank">
        Supported by Rakuten Developers
      </a>
    </p>
  )
}

// 新刊を押したときの小さなダイアログ。作品を記録するか、楽天の商品ページを見るかを選ぶ。
//
// 楽天の本は AniList の作品と結び付けていないので（BACKLOG 11-b）、「作品を探して記録する」は
// 書名から巻数を落とした作品名で検索画面を開くだけにする。記録するのは AniList の作品で、楽天の情報は保存しない。
// 文言は「購入」を促さない形にしている。規約（第 10 条）がクリックを促す表示を禁じているため
function BookDialog({ book, onClose }: { book: NewReleaseItem; onClose: () => void }) {
  const navigate = useNavigate()

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const series = seriesTitleOf(book.title)
  const meta = [
    book.author,
    book.publisherName,
    `${formatMonthDay(book.salesDate)}発売${book.salesDateApproximate ? '予定' : ''}`,
    book.genre,
  ].filter(Boolean)

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="card elev-lg"
        style={{ width: 420, maxWidth: '100%', padding: 'var(--space-6)', position: 'relative' }}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="book-dialog-title"
      >
        <button
          type="button"
          className="btn btn-icon"
          style={{ position: 'absolute', top: 12, right: 12 }}
          onClick={onClose}
          aria-label="閉じる"
        >
          ✕
        </button>
        <div style={{ display: 'flex', gap: 'var(--space-4)' }}>
          <Thumb item={{ title: book.title, coverImageUrl: book.imageUrl }} fontSize={26} style={{ width: '96px', aspectRatio: '2/3', flex: 'none' }} />
          <div style={{ minWidth: 0, paddingRight: 28 }}>
            <h3 id="book-dialog-title" style={{ margin: '0 0 6px', fontSize: 17, lineHeight: 1.4 }}>
              {book.title}
            </h3>
            <div className="text-muted" style={{ fontSize: 12, lineHeight: 1.7 }}>
              {meta.join('・')}
            </div>
          </div>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 'var(--space-6)' }}>
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => navigate(`/search?kind=manga&q=${encodeURIComponent(series)}`)}
          >
            作品を探して記録する
          </button>
          <a className="btn btn-secondary" href={book.url} target="_blank" rel="noopener noreferrer" style={{ textAlign: 'center' }}>
            楽天ブックスで見る
          </a>
        </div>
        <p className="text-muted" style={{ fontSize: 11, margin: 'var(--space-3) 0 0' }}>
          「{series}」で作品を検索します。
        </p>
      </div>
    </div>
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
