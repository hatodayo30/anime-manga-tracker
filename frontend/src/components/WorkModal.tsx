import { useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { api } from '../lib/api'
import { STATUS_LABELS, activateOnKey, colorForTitle, formatScore, translateGenre, translateRelation, unitFor, type MediaKind, type Status } from '../lib/util'
import type { LibraryRecord, RelatedWork } from '../types'
import { Thumb } from './Thumb'
import type { OpenWorkModalOptions, WorkModalItem } from './WorkModalContext'

interface FreshInfo {
  total: number | null // アニメ=話数 / 漫画=巻数
  airingStatus?: string
  nextEpisode?: number | null
}

interface Props {
  item: WorkModalItem
  mediaType: MediaKind
  record: LibraryRecord | null
  onChange?: () => void
  onClose: () => void
  openRelated: (opts: OpenWorkModalOptions) => void
}

export function WorkModal({ item, mediaType, record, onChange, onClose, openRelated }: Props) {
  const { user } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  const [currentRecord, setCurrentRecord] = useState(record)
  const [pending, setPending] = useState(false)
  const [editingProgress, setEditingProgress] = useState(false)
  const [confirmingRemove, setConfirmingRemove] = useState(false)
  const [ratingMemoOpen, setRatingMemoOpen] = useState(() => !!(record?.rating || record?.memo))
  const [translatedSynopsis, setTranslatedSynopsis] = useState<string | null>(null)
  const [freshInfo, setFreshInfo] = useState<FreshInfo | null>(null)
  const [relatedWorks, setRelatedWorks] = useState<RelatedWork[] | null>(null)

  // 依存配列は意図的に空: このコンポーネントは開いた作品ごとにkeyで再マウントされるため、
  // マウント時に一度だけ取得すればよい（item/mediaTypeがこのインスタンス内で変わることはない）。
  useEffect(() => {
    let cancelled = false
    if (item.anilistId) {
      api
        .mediaByIds({ type: mediaType, ids: [item.anilistId] })
        .then((results) => {
          if (cancelled || results.length === 0) return
          setFreshInfo(results[0])
        })
        .catch(() => {})
    }
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    if (item.anilistId) {
      api
        .relations({ id: item.anilistId })
        .then((results) => {
          if (cancelled) return
          setRelatedWorks(results)
        })
        .catch(() => {})
    }
    return () => {
      cancelled = true
    }
  }, [])

  // AniListの最新情報と保存済みの総数（アニメ=話数 / 漫画=巻数）がズレていたら同期する。
  // 進捗バーと「全巻/全話読了でdoneへ」の判定がDBの値だけを見るライブラリ画面でも
  // 正しく効くようにするため。漫画の記録単位を話数から巻数へ切り替えた既存レコードも
  // モーダルを開いた時点でここで巻数に移行される。
  useEffect(() => {
    const fresh = freshInfo?.total
    if (!currentRecord || fresh == null || fresh === currentRecord.total) return

    let cancelled = false
    api
      .updateRecord(currentRecord.id, { total: fresh })
      .then((next) => {
        if (cancelled) return
        setCurrentRecord(next)
        onChange?.()
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [freshInfo, currentRecord])

  useEffect(() => {
    let cancelled = false
    if (item.synopsis) {
      api
        .translate({ text: item.synopsis, target: 'ja' })
        .then((translated) => {
          if (cancelled || !translated) return
          setTranslatedSynopsis(translated)
        })
        .catch(() => {})
    }
    return () => {
      cancelled = true
    }
  }, [])

  const setStatus = async (newStatus: Status) => {
    if (!user) {
      navigate(`/login?next=${encodeURIComponent(location.pathname)}`)
      return
    }
    if (pending) return
    setPending(true)
    try {
      const next = currentRecord
        ? await api.updateRecord(currentRecord.id, { status: newStatus })
        : await api.createRecord({
            anilistId: item.anilistId,
            mediaType,
            title: item.title,
            coverImageUrl: item.coverImageUrl || '',
            genres: item.genres || [],
            total: item.total ?? null,
            status: newStatus,
            nextAiringAt: typeof item.nextAiringAt === 'number' ? item.nextAiringAt : null,
          })
      setCurrentRecord(next)
      onChange?.()
    } catch (err) {
      alert(`保存に失敗しました: ${err instanceof Error ? err.message : err}`)
    }
    setPending(false)
  }

  const commitProgress = async (rawN: number, total: number | null, progressCap: number | null) => {
    if (!currentRecord || pending) return
    let n = Math.max(0, rawN)
    if (progressCap != null) n = Math.min(n, progressCap)
    if (n === currentRecord.progress) return

    setPending(true)
    const body: { progress: number; status?: Status } = { progress: n }
    if (total != null && n >= total) body.status = 'done'
    try {
      const next = await api.updateRecord(currentRecord.id, body)
      setCurrentRecord(next)
      onChange?.()
    } catch (err) {
      alert(`更新に失敗しました: ${err instanceof Error ? err.message : err}`)
    }
    setPending(false)
  }

  const adjustProgress = (delta: number, total: number | null, progressCap: number | null) => {
    if (!currentRecord) return
    commitProgress(currentRecord.progress + delta, total, progressCap)
  }

  const saveRating = async (n: number) => {
    if (!currentRecord || pending) return
    setPending(true)
    try {
      const next = await api.updateRecord(currentRecord.id, { rating: n })
      setCurrentRecord(next)
      onChange?.()
    } catch (err) {
      alert(`評価の保存に失敗しました: ${err instanceof Error ? err.message : err}`)
    }
    setPending(false)
  }

  const saveMemo = async (text: string) => {
    if (!currentRecord || pending || text === currentRecord.memo) return
    setPending(true)
    try {
      const next = await api.updateRecord(currentRecord.id, { memo: text })
      setCurrentRecord(next)
      onChange?.()
    } catch (err) {
      alert(`メモの保存に失敗しました: ${err instanceof Error ? err.message : err}`)
    }
    setPending(false)
  }

  const removeFromLibrary = async () => {
    if (!currentRecord || pending) return
    setPending(true)
    try {
      await api.deleteRecord(currentRecord.id)
      setCurrentRecord(null)
      onChange?.()
    } catch (err) {
      alert(`削除に失敗しました: ${err instanceof Error ? err.message : err}`)
    }
    setPending(false)
    setConfirmingRemove(false)
  }

  const openRelatedWorkModal = (work: RelatedWork) => {
    openRelated({
      item: { anilistId: work.anilistId, title: work.title, coverImageUrl: work.coverImageUrl, genres: [] },
      mediaType: work.mediaType,
      record: null,
      onChange,
    })
  }

  const labels = STATUS_LABELS[mediaType]
  const unit = unitFor(mediaType)
  const total = freshInfo?.total ?? item.total ?? currentRecord?.total ?? null
  const status = currentRecord?.status ?? null
  const airingEp =
    mediaType === 'anime' && freshInfo?.airingStatus === 'RELEASING' && freshInfo?.nextEpisode != null
      ? freshInfo.nextEpisode - 1
      : null
  const progressCap = airingEp != null ? airingEp : total

  const scoreLabel = formatScore(item.score)
  const metaParts = [mediaType === 'anime' ? 'アニメ' : '漫画', total ? `全${total}${unit}` : '連載中']
  if (scoreLabel) metaParts.push(`★${scoreLabel}`)

  // 漫画の既刊巻数は total（=AniListのvolumes）として metaParts に出るため、ここでは扱わない。
  const supplementalInfo = airingEp != null ? `現在${airingEp}${unit}放送中` : null

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="card elev-lg"
        style={{ width: 660, maxWidth: '100%', maxHeight: '90vh', overflowY: 'auto', padding: 0, position: 'relative' }}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="work-modal-title"
      >
        <div
          className="dialog-banner"
          style={{ background: `linear-gradient(135deg, ${colorForTitle(item.title)} 0%, var(--color-surface) 92%)` }}
        >
          <button
            type="button"
            className="btn btn-icon"
            style={{ position: 'absolute', top: 12, right: 12, background: 'var(--color-surface)' }}
            onClick={onClose}
          >
            ✕
          </button>
        </div>

        <div style={{ display: 'flex', gap: 'var(--space-6)', padding: '0 var(--space-6) var(--space-6)', marginTop: -56 }}>
          <div style={{ width: 132, flex: 'none' }}>
            <Thumb item={item} fontSize={38} style={{ width: '132px', aspectRatio: '2/3', boxShadow: 'var(--shadow-md)' }} />
          </div>
          <div style={{ flex: 1, minWidth: 0, paddingTop: 60 }}>
            <h3 id="work-modal-title" style={{ margin: '0 0 6px', fontSize: 21, lineHeight: 1.3 }}>
              {item.title}
            </h3>
            <div className="text-muted" style={{ fontSize: 12, marginBottom: 10 }}>
              {metaParts.join('・')}
              {supplementalInfo ? ` ・ ${supplementalInfo}` : ''}
            </div>
            <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap', marginBottom: 12 }}>
              {(item.genres || []).map((g) => (
                <span key={g} className="tag tag-outline" style={{ fontSize: 11 }}>
                  {translateGenre(g)}
                </span>
              ))}
            </div>
            <p
              style={{
                fontSize: 13,
                lineHeight: 1.75,
                margin: 0,
                display: '-webkit-box',
                WebkitLineClamp: 4,
                WebkitBoxOrient: 'vertical',
                overflow: 'hidden',
              }}
            >
              {translatedSynopsis || item.synopsis || 'あらすじは未登録です。'}
            </p>
          </div>
        </div>

        <div style={{ padding: '0 var(--space-6) var(--space-6)' }}>
          <div style={{ borderTop: '1px solid var(--color-divider)', paddingTop: 'var(--space-6)' }}>
            <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: 10 }}>
              <span className="text-muted" style={{ fontSize: 12 }}>
                記録する
              </span>
              {currentRecord &&
                (confirmingRemove ? (
                  <span style={{ fontSize: 11, display: 'inline-flex', gap: 8, alignItems: 'center' }}>
                    本当に外しますか？
                    <button type="button" className="link-btn" style={{ fontSize: 11 }} onClick={removeFromLibrary}>
                      はい
                    </button>
                    <button type="button" className="link-btn" style={{ fontSize: 11 }} onClick={() => setConfirmingRemove(false)}>
                      いいえ
                    </button>
                  </span>
                ) : (
                  <button type="button" className="link-btn" style={{ fontSize: 11 }} onClick={() => setConfirmingRemove(true)}>
                    記録から外す
                  </button>
                ))}
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 8 }}>
              {(['done', 'active', 'want'] as const).map((key) => (
                <button
                  key={key}
                  type="button"
                  className={`status-btn ${status === key ? 'on' : ''}`}
                  disabled={pending}
                  onClick={() => setStatus(key)}
                >
                  {status === key ? `✓ ${labels[key]}` : labels[key]}
                </button>
              ))}
            </div>

            {status === 'active' && currentRecord && (
              <ProgressRow
                progress={currentRecord.progress}
                total={total}
                progressCap={progressCap}
                pending={pending}
                editingProgress={editingProgress}
                unit={unit}
                onEdit={() => setEditingProgress(true)}
                onCancelEdit={() => setEditingProgress(false)}
                onSubmitEdit={(raw) => {
                  setEditingProgress(false)
                  const n = parseInt(raw, 10)
                  if (!Number.isNaN(n)) commitProgress(n, total, progressCap)
                }}
                onAdjust={(delta) => adjustProgress(delta, total, progressCap)}
              />
            )}

            {currentRecord && (
              <div style={{ marginTop: 'var(--space-4)', fontSize: 12, color: 'var(--color-accent)' }}>
                ✓ 「{labels[currentRecord.status]}」に記録しました
              </div>
            )}
          </div>
        </div>

        {currentRecord && (
          <div style={{ padding: '0 var(--space-6) var(--space-6)' }}>
            <div style={{ borderTop: '1px solid var(--color-divider)', paddingTop: 'var(--space-6)' }}>
              <div
                style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', cursor: 'pointer' }}
                role="button"
                tabIndex={0}
                aria-expanded={ratingMemoOpen}
                onClick={() => setRatingMemoOpen((v) => !v)}
                onKeyDown={activateOnKey(() => setRatingMemoOpen((v) => !v))}
              >
                <span className="text-muted" style={{ fontSize: 12 }}>
                  評価・感想
                  {!ratingMemoOpen && currentRecord.rating ? ` ・ ${'★'.repeat(currentRecord.rating)}` : ''}
                </span>
                <button type="button" className="link-btn" style={{ fontSize: 11 }}>
                  {ratingMemoOpen ? '閉じる' : currentRecord.rating || currentRecord.memo ? '編集' : '＋ 記録する'}
                </button>
              </div>

              {ratingMemoOpen && (
                <div style={{ marginTop: 'var(--space-4)' }}>
                  <StarRating value={currentRecord.rating} disabled={pending} onChange={saveRating} />
                  <textarea
                    key={currentRecord.id}
                    className="input"
                    style={{ marginTop: 'var(--space-3)', minHeight: 64, borderRadius: 'var(--radius-md)', resize: 'vertical', fontFamily: 'inherit' }}
                    placeholder="一言メモ（感想など）"
                    defaultValue={currentRecord.memo}
                    disabled={pending}
                    onBlur={(e) => saveMemo(e.target.value)}
                  />
                </div>
              )}
            </div>
          </div>
        )}

        {relatedWorks && relatedWorks.length > 0 && (
          <div style={{ padding: '0 var(--space-6) var(--space-6)' }}>
            <div style={{ borderTop: '1px solid var(--color-divider)', paddingTop: 'var(--space-6)' }}>
              <div className="text-muted" style={{ fontSize: 12, marginBottom: 10 }}>
                関連作品
              </div>
              <div style={{ display: 'flex', gap: 10, overflowX: 'auto', paddingBottom: 4 }}>
                {relatedWorks.map((work) => (
                  <div
                    key={work.anilistId}
                    style={{ flex: 'none', width: 84, cursor: 'pointer' }}
                    onClick={() => openRelatedWorkModal(work)}
                    role="button"
                    tabIndex={0}
                    onKeyDown={activateOnKey(() => openRelatedWorkModal(work))}
                  >
                    <Thumb item={work} fontSize={22} style={{ width: '84px', height: '112px' }} />
                    <div className="tag tag-outline" style={{ fontSize: 10, padding: '2px 8px', margin: '6px 0 3px' }}>
                      {translateRelation(work.relationType)}
                    </div>
                    <div style={{ fontSize: 11, lineHeight: 1.3, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {work.title}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// 星をタップして評価(1〜5)をセットする。現在の評価と同じ星を再タップすると評価を解除する(0を送信)。
function StarRating({ value, disabled, onChange }: { value: number | null; disabled: boolean; onChange: (n: number) => void }) {
  return (
    <div role="radiogroup" aria-label="評価" style={{ display: 'flex', gap: 2 }}>
      {[1, 2, 3, 4, 5].map((n) => {
        const filled = value != null && n <= value
        return (
          <button
            key={n}
            type="button"
            disabled={disabled}
            role="radio"
            aria-checked={value === n}
            aria-label={`★${n}`}
            onClick={() => onChange(value === n ? 0 : n)}
            style={{
              background: 'none',
              border: 'none',
              padding: 3,
              lineHeight: 1,
              fontSize: 22,
              cursor: disabled ? 'default' : 'pointer',
              color: filled ? 'var(--color-accent)' : 'var(--color-neutral-800)',
            }}
          >
            {filled ? '★' : '☆'}
          </button>
        )
      })}
    </div>
  )
}

function ProgressRow({
  progress,
  total,
  progressCap,
  pending,
  editingProgress,
  unit,
  onEdit,
  onCancelEdit,
  onSubmitEdit,
  onAdjust,
}: {
  progress: number
  total: number | null
  progressCap: number | null
  pending: boolean
  editingProgress: boolean
  unit: string
  onEdit: () => void
  onCancelEdit: () => void
  onSubmitEdit: (raw: string) => void
  onAdjust: (delta: number) => void
}) {
  const pct = total ? Math.min(100, Math.round((progress / total) * 100)) : 0

  return (
    <div style={{ marginTop: 'var(--space-4)', display: 'flex', alignItems: 'center', gap: 'var(--space-4)', flexWrap: 'wrap' }}>
      <span className="text-muted" style={{ fontSize: 12 }}>
        進捗
      </span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <button
          type="button"
          className="btn btn-icon"
          aria-label="進捗を1減らす"
          disabled={pending || progress <= 0}
          onClick={() => onAdjust(-1)}
        >
          －
        </button>
        {editingProgress ? (
          <ProgressEditInput progress={progress} progressCap={progressCap} pending={pending} unit={unit} onCancel={onCancelEdit} onSubmit={onSubmitEdit} />
        ) : (
          <span
            style={{
              fontFamily: 'var(--font-heading)',
              fontSize: 17,
              fontWeight: 600,
              minWidth: 86,
              textAlign: 'center',
              cursor: pending ? 'default' : 'pointer',
            }}
            title="タップして直接入力"
            role="button"
            tabIndex={pending ? undefined : 0}
            aria-label="進捗数を直接入力する"
            onClick={() => !pending && onEdit()}
            onKeyDown={pending ? undefined : activateOnKey(onEdit)}
          >
            {total ? `${progress} / ${total}${unit}` : `${progress}${unit} / ？${unit}`}
          </span>
        )}
        <button
          type="button"
          className="btn btn-icon"
          aria-label="進捗を1増やす"
          disabled={pending || (progressCap != null && progress >= progressCap)}
          onClick={() => onAdjust(1)}
        >
          ＋
        </button>
      </div>
      {total ? (
        <div style={{ flex: 1, minWidth: 120, height: 4, background: 'var(--color-neutral-800)', borderRadius: 2, overflow: 'hidden' }}>
          <div style={{ height: '100%', background: 'var(--color-accent)', width: `${pct}%` }} />
        </div>
      ) : null}
    </div>
  )
}

// 進捗の数値をタップして手動入力するときの入力欄。Escapeでキャンセルした場合、
// そのあとに発生するblurイベントでは値を確定させない（cancelledRefで抑止する）。
function ProgressEditInput({
  progress,
  progressCap,
  pending,
  unit,
  onCancel,
  onSubmit,
}: {
  progress: number
  progressCap: number | null
  pending: boolean
  unit: string
  onCancel: () => void
  onSubmit: (raw: string) => void
}) {
  const cancelledRef = useRef(false)

  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, minWidth: 86, justifyContent: 'center' }}>
      <input
        type="number"
        className="input input-progress-edit"
        aria-label="進捗数を直接入力"
        min={0}
        max={progressCap ?? undefined}
        defaultValue={progress}
        disabled={pending}
        autoFocus
        onFocus={(e) => e.target.select()}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.currentTarget.blur()
          } else if (e.key === 'Escape') {
            cancelledRef.current = true
            onCancel()
          }
        }}
        onBlur={(e) => {
          if (cancelledRef.current) return
          onSubmit(e.target.value)
        }}
      />
      {unit}
    </span>
  )
}
