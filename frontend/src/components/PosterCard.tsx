import { Thumb } from './Thumb'

interface PosterItem {
  title: string
  coverImageUrl?: string | null
}

export function PosterCard({
  item,
  kindLabel,
  badgeLabel,
  caption,
  pct,
  ranked,
  onClick,
}: {
  item: PosterItem
  kindLabel?: string | null
  badgeLabel?: string | null
  caption?: string | null
  pct?: number | null
  ranked?: number
  onClick?: () => void
}) {
  return (
    <div className={`poster-card ${ranked != null ? 'poster-card-ranked' : ''}`} onClick={onClick}>
      {ranked != null && <span className="poster-rank-num">{ranked}</span>}
      <Thumb item={item} fontSize={26} className="poster-thumb">
        {kindLabel && <span className="poster-kindbadge">{kindLabel}</span>}
        {badgeLabel && <span className="poster-badge">{badgeLabel}</span>}
      </Thumb>
      <div className="poster-title">{item.title}</div>
      {pct != null && (
        <div className="poster-bar">
          <div className="poster-bar-fill" style={{ width: `${pct}%` }} />
        </div>
      )}
      {caption && (
        <div className="text-muted" style={{ fontSize: 11, marginTop: 4 }}>
          {caption}
        </div>
      )}
    </div>
  )
}
