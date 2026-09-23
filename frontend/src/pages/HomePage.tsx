import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { PosterCard } from '../components/PosterCard'
import { KindToggle } from '../components/KindToggle'
import { Thumb } from '../components/Thumb'
import { useWorkModal } from '../components/WorkModalContext'
import { useAuth } from '../context/AuthContext'
import { useKind } from '../hooks/useKind'
import { api } from '../lib/api'
import {
  STATUS_LABELS,
  WEEKDAY_EN_SHORT,
  WEEKDAY_JA,
  formatScore,
  formatWeekday,
  jstAiringInfo,
  unitFor,
  type MediaKind,
} from '../lib/util'
import type { AniListItem, LibraryRecord, MangaRankingItem } from '../types'

export function HomePage() {
  const { user } = useAuth()
  const [kind, setKind] = useKind()
  const [season, setSeason] = useState<AniListItem[]>([])
  const [seasonError, setSeasonError] = useState<string | null>(null)
  const [ranking, setRanking] = useState<MangaRankingItem[]>([])
  const [rankingError, setRankingError] = useState<string | null>(null)
  const [library, setLibrary] = useState<LibraryRecord[]>([])
  const [reloadSeq, setReloadSeq] = useState(0)
  const rankingCache = useRef<MangaRankingItem[] | null>(null)
  const openWorkModal = useWorkModal()

  const reload = () => setReloadSeq((n) => n + 1)

  useEffect(() => {
    let cancelled = false

    if (kind === 'anime') {
      api
        .seasonAnime()
        .then((r) => !cancelled && (setSeason(r), setSeasonError(null)))
        .catch((err) => !cancelled && setSeasonError(err instanceof Error ? err.message : String(err)))
    } else {
      setSeason([])
    }

    if (kind === 'manga') {
      if (rankingCache.current) {
        setRanking(rankingCache.current)
      } else {
        api
          .trendingManga()
          .then((r) => {
            if (cancelled) return
            rankingCache.current = r
            setRanking(r)
            setRankingError(null)
          })
          .catch((err) => !cancelled && setRankingError(err instanceof Error ? err.message : String(err)))
      }
    } else {
      setRanking([])
    }

    if (user) {
      api
        .listRecords({ type: kind })
        .then((r) => !cancelled && setLibrary(r))
        .catch(() => !cancelled && setLibrary([]))
    } else {
      setLibrary([])
    }

    return () => {
      cancelled = true
    }
  }, [kind, user, reloadSeq])

  const libraryByAniListId = new Map(library.map((r) => [r.anilistId, r]))

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-4)', marginBottom: 'var(--space-6)', flexWrap: 'wrap' }}>
        <h2 style={{ margin: 0 }}>ホーム</h2>
        <KindToggle kind={kind} onChange={setKind} />
      </div>

      <ContinueShelf kind={kind} user={!!user} library={library} onOpen={(r) => openWorkModal({ item: r, mediaType: kind, record: r, onChange: reload })} />

      {kind === 'anime' && (
        <ScheduleShelf season={season} library={libraryByAniListId} onOpen={(item) => openWorkModal({ item, mediaType: 'anime', record: libraryByAniListId.get(item.anilistId) || null, onChange: reload })} />
      )}

      {kind === 'anime' && (
        <div className="shelf">
          <div className="shelf-head">
            <h5 className="shelf-h">今季放送中アニメ</h5>
            <Link to="/search" className="shelf-more">
              すべて ›
            </Link>
          </div>
          {seasonError ? (
            <p className="text-muted">AniListからの取得に失敗しました: {seasonError}</p>
          ) : (
            <div className="poster-row">
              {season.map((item) => {
                const r = libraryByAniListId.get(item.anilistId)
                return (
                  <PosterCard
                    key={item.anilistId}
                    item={item}
                    badgeLabel={r ? `${STATUS_LABELS.anime[r.status]} ✓` : null}
                    caption={formatWeekday(item.nextAiringAt) || '放送日未定'}
                    onClick={() => openWorkModal({ item, mediaType: 'anime', record: r || null, onChange: reload })}
                  />
                )
              })}
            </div>
          )}
        </div>
      )}

      {kind === 'manga' && (
        <div className="shelf">
          <div className="shelf-head">
            <h5 className="shelf-h">話題の漫画 TOP10</h5>
            <Link to="/search" className="shelf-more">
              すべて ›
            </Link>
          </div>
          {rankingError ? (
            <p className="text-muted">ランキングの取得に失敗しました: {rankingError}</p>
          ) : ranking.length === 0 ? (
            <p className="text-muted">ランキング情報を取得できませんでした。</p>
          ) : (
            <div className="poster-row">
              {ranking.map((item, i) => (
                <RankingPoster key={item.anilistId} item={item} rank={i + 1} onClick={() => openWorkModal({ item, mediaType: 'manga', record: libraryByAniListId.get(item.anilistId) || null, onChange: reload })} />
              ))}
            </div>
          )}
        </div>
      )}

      {kind === 'manga' && <MagazineShelf ranking={ranking} onOpen={(item) => openWorkModal({ item, mediaType: 'manga', record: libraryByAniListId.get(item.anilistId) || null, onChange: reload })} />}

      <RecommendPreviewShelf kind={kind} user={!!user} library={library} onOpen={(item) => openWorkModal({ item, mediaType: kind, record: null, onChange: reload })} />
    </>
  )
}

function ContinueShelf({
  kind,
  user,
  library,
  onOpen,
}: {
  kind: MediaKind
  user: boolean
  library: LibraryRecord[]
  onOpen: (r: LibraryRecord) => void
}) {
  const unit = unitFor()
  const title = kind === 'anime' ? 'つづきを見る' : 'つづきを読む'

  return (
    <div className="shelf">
      <div className="shelf-head">
        <h5 className="shelf-h">{title}</h5>
        <Link to="/library" className="shelf-more">
          すべて ›
        </Link>
      </div>
      {!user ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          ログインすると、記録中の作品からつづきを再開できます。 <Link to="/login">ログイン</Link>
        </p>
      ) : (
        (() => {
          const active = library.filter((r) => r.status === 'active')
          if (active.length === 0) {
            return (
              <p className="text-muted" style={{ fontSize: 13 }}>
                記録中の{kind === 'anime' ? 'アニメ' : '漫画'}はまだありません。検索から追加できます。
              </p>
            )
          }
          return (
            <div className="poster-row">
              {active.map((r) => {
                const pct = r.total ? Math.min(100, Math.round((r.progress / r.total) * 100)) : 0
                const caption = r.total ? `${r.progress} / ${r.total}${unit}` : `${r.progress}${unit}まで`
                return <PosterCard key={r.id} item={r} pct={pct} caption={caption} onClick={() => onOpen(r)} />
              })}
            </div>
          )
        })()
      )}
    </div>
  )
}

function ScheduleShelf({
  season,
  library,
  onOpen,
}: {
  season: AniListItem[]
  library: Map<number, LibraryRecord>
  onOpen: (item: AniListItem) => void
}) {
  const withSchedule = season.filter((a) => a.nextAiringAt)

  return (
    <div className="shelf">
      <div className="shelf-head">
        <h5 className="shelf-h">今週の放送予定</h5>
      </div>
      {withSchedule.length === 0 ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          放送予定の情報がありません。
        </p>
      ) : (
        <ScheduleDays withSchedule={withSchedule} library={library} onOpen={onOpen} />
      )}
    </div>
  )
}

function ScheduleDays({
  withSchedule,
  library,
  onOpen,
}: {
  withSchedule: AniListItem[]
  library: Map<number, LibraryRecord>
  onOpen: (item: AniListItem) => void
}) {
  const todayName = new Intl.DateTimeFormat('en-US', { timeZone: 'Asia/Tokyo', weekday: 'short' }).format(new Date())
  const todayIndex = WEEKDAY_EN_SHORT.indexOf(todayName)

  const byDay = new Map<number, { item: AniListItem; hour: number; minute: string }[]>()
  for (const a of withSchedule) {
    const info = jstAiringInfo(a.nextAiringAt)
    if (!info) continue
    if (!byDay.has(info.dayIndex)) byDay.set(info.dayIndex, [])
    byDay.get(info.dayIndex)!.push({ item: a, hour: info.hour, minute: info.minute })
  }
  const days = Array.from(byDay.keys()).sort((a, b) => a - b)

  return (
    <div className="poster-row">
      {days.map((dayIndex) => {
        const items = byDay.get(dayIndex)!.sort((a, b) => a.hour - b.hour)
        const isToday = dayIndex === todayIndex
        return (
          <div
            key={dayIndex}
            className="card elev-sm"
            style={{ flex: 'none', width: 150, padding: 'var(--space-3)', gap: 10, borderColor: isToday ? 'var(--color-accent)' : undefined }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
              <span style={{ fontFamily: 'var(--font-heading)', fontSize: 14, fontWeight: 600, color: isToday ? 'var(--color-accent)' : 'var(--color-text)' }}>
                {isToday ? `${WEEKDAY_JA[dayIndex]} 今日` : WEEKDAY_JA[dayIndex]}
              </span>
              <span className="text-muted" style={{ fontSize: 11 }}>
                {items.length}件
              </span>
            </div>
            {items.map(({ item, hour, minute }) => (
              <div key={item.anilistId} style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }} onClick={() => onOpen(item)}>
                <Thumb item={item} width="24px" height="32px" fontSize={12} />
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontSize: 11, lineHeight: 1.3, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{item.title}</div>
                  <div className="text-muted" style={{ fontSize: 10, marginTop: 2 }}>
                    {hour}:{minute}
                    {library.has(item.anilistId) ? ' ・ 記録済み' : ''}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )
      })}
    </div>
  )
}

function RankingPoster({ item, rank, onClick }: { item: MangaRankingItem; rank: number; onClick: () => void }) {
  const score = formatScore(item.score)
  const captionParts = [score ? `★${score}` : null, item.malRank ? `MAL #${item.malRank}` : null].filter(Boolean)

  return (
    <div className="poster-card poster-card-ranked" onClick={onClick}>
      <span className="poster-rank-num">{rank}</span>
      <Thumb item={item} fontSize={26} className="poster-thumb" />
      <div className="poster-title">{item.title}</div>
      {captionParts.length > 0 && (
        <div className="text-muted" style={{ fontSize: 11, marginTop: 4 }}>
          {captionParts.join(' ・ ')}
        </div>
      )}
      {item.members ? (
        <div className="text-muted" style={{ fontSize: 11, marginTop: 2 }}>
          {Math.round(item.members / 1000)}千人が記録
        </div>
      ) : null}
    </div>
  )
}

function MagazineShelf({ ranking, onOpen }: { ranking: MangaRankingItem[]; onOpen: (item: MangaRankingItem) => void }) {
  const [activeMagazine, setActiveMagazine] = useState<string | null>(null)

  const counts = new Map<string, number>()
  for (const item of ranking) {
    for (const mag of item.magazines || []) counts.set(mag, (counts.get(mag) || 0) + 1)
  }
  const magazines = Array.from(counts.keys()).sort((a, b) => (counts.get(b) ?? 0) - (counts.get(a) ?? 0))

  if (magazines.length === 0) {
    return (
      <div className="shelf">
        <div className="shelf-head">
          <h5 className="shelf-h">掲載誌で探す</h5>
        </div>
        <p className="text-muted" style={{ fontSize: 13 }}>
          掲載誌の情報を取得できませんでした。
        </p>
      </div>
    )
  }

  const active = activeMagazine && counts.has(activeMagazine) ? activeMagazine : magazines[0]
  const works = ranking.filter((item) => (item.magazines || []).includes(active))

  return (
    <div className="shelf">
      <div className="shelf-head">
        <h5 className="shelf-h">掲載誌で探す</h5>
      </div>
      <p className="text-muted shelf-desc">{active} に載っている作品</p>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 'var(--space-3)' }}>
        {magazines.map((mag) => (
          <button
            key={mag}
            type="button"
            className={`tag ${mag === active ? 'tag-accent' : 'tag-outline'}`}
            onClick={() => setActiveMagazine(mag)}
          >
            {mag} ({counts.get(mag)})
          </button>
        ))}
      </div>
      <div className="poster-row">
        {works.map((item) => {
          const score = formatScore(item.score)
          return <PosterCard key={item.anilistId} item={item} caption={score ? `★${score}` : null} onClick={() => onOpen(item)} />
        })}
      </div>
    </div>
  )
}

function RecommendPreviewShelf({
  kind,
  user,
  library,
  onOpen,
}: {
  kind: MediaKind
  user: boolean
  library: LibraryRecord[]
  onOpen: (item: AniListItem) => void
}) {
  const [preview, setPreview] = useState<AniListItem[] | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (!user) return
    let cancelled = false
    setPreview(null)
    setFailed(false)

    const counts = new Map<string, number>()
    for (const r of library) {
      if (r.status !== 'done' && r.status !== 'active') continue
      for (const g of r.genres) counts.set(g, (counts.get(g) || 0) + 1)
    }
    const genres = Array.from(counts.entries()).sort((a, b) => b[1] - a[1]).slice(0, 3).map(([g]) => g)

    if (genres.length === 0) {
      setPreview([])
      return
    }

    api
      .recommendations({ type: kind, genres })
      .then((results) => {
        if (cancelled) return
        const registeredIds = new Set(library.map((r) => r.anilistId))
        setPreview(results.filter((item) => !registeredIds.has(item.anilistId)).slice(0, 6))
      })
      .catch(() => {
        if (cancelled) return
        setFailed(true)
      })

    return () => {
      cancelled = true
    }
  }, [kind, user, library])

  return (
    <div className="shelf">
      <div className="shelf-head">
        <h5 className="shelf-h">あなたへのおすすめ</h5>
        <Link to="/recommend" className="shelf-more">
          すべて ›
        </Link>
      </div>
      {!user ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          ログインすると、あなたの好みに合わせたおすすめが表示されます。 <Link to="/login">ログイン</Link>
        </p>
      ) : failed ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          おすすめの取得に失敗しました。
        </p>
      ) : preview == null ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          読み込み中…
        </p>
      ) : preview.length === 0 ? (
        <p className="text-muted" style={{ fontSize: 13 }}>
          記録が増えると、あなたの好みに合わせたおすすめが表示されます。
        </p>
      ) : (
        <div className="poster-row">
          {preview.map((item) => {
            const score = formatScore(item.score)
            return <PosterCard key={item.anilistId} item={item} caption={score ? `★${score}` : null} onClick={() => onOpen(item)} />
          })}
        </div>
      )}
    </div>
  )
}
