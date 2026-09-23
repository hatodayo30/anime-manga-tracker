import type { MediaKind } from '../lib/util'

export function KindToggle({ kind, onChange }: { kind: MediaKind; onChange: (kind: MediaKind) => void }) {
  return (
    <div className="seg" style={{ flex: 'none', marginBottom: 0 }}>
      {(['anime', 'manga'] as const).map((k) => (
        <button
          key={k}
          type="button"
          className={`seg-opt${kind === k ? ' checked' : ''}`}
          onClick={() => onChange(k)}
        >
          {k === 'anime' ? 'アニメ' : '漫画'}
        </button>
      ))}
    </div>
  )
}
