import { useState } from 'react';
import type { ItemType, LibraryItem, Status } from '../types';
import { STATUS_LABELS } from '../data/sampleData';

interface LibraryProps {
  library: LibraryItem[];
  setProgress: (id: number, val: string) => void;
  showProgressBar?: boolean;
}

export function Library({ library, setProgress, showProgressBar = true }: LibraryProps) {
  const [libType, setLibType] = useState<ItemType>('anime');
  const [libStatus, setLibStatus] = useState<Status>('active');
  const [activeGenres, setActiveGenres] = useState<string[]>([]);
  const [editingProgressId, setEditingProgressId] = useState<number | null>(null);

  const labels = STATUS_LABELS[libType];
  const pool = library.filter((i) => i.type === libType && i.status === libStatus);
  const allGenres = Array.from(new Set(library.filter((i) => i.type === libType).flatMap((i) => i.genres)));
  const filtered = activeGenres.length ? pool.filter((i) => i.genres.some((g) => activeGenres.includes(g))) : pool;

  const toggleGenre = (g: string) => {
    setActiveGenres((prev) => (prev.includes(g) ? prev.filter((x) => x !== g) : [...prev, g]));
  };

  const unit = (type: ItemType) => (type === 'anime' ? '話' : '巻');

  return (
    <div style={{ maxWidth: 920 }}>
      <h2 style={{ marginBottom: 4 }}>マイライブラリ</h2>
      <p className="text-muted" style={{ marginBottom: 'var(--space-6)', fontSize: 13 }}>
        ステータス別に記録を確認
      </p>

      <div className="seg" style={{ marginBottom: 'var(--space-3)' }}>
        <label className="seg-opt">
          <input type="radio" name="libtype" checked={libType === 'anime'} onChange={() => { setLibType('anime'); setActiveGenres([]); }} />
          アニメ
        </label>
        <label className="seg-opt">
          <input type="radio" name="libtype" checked={libType === 'manga'} onChange={() => { setLibType('manga'); setActiveGenres([]); }} />
          漫画
        </label>
      </div>

      <div className="seg" style={{ marginBottom: 'var(--space-4)' }}>
        <label className="seg-opt">
          <input type="radio" name="libstatus" checked={libStatus === 'done'} onChange={() => setLibStatus('done')} />
          {labels.done}
        </label>
        <label className="seg-opt">
          <input type="radio" name="libstatus" checked={libStatus === 'active'} onChange={() => setLibStatus('active')} />
          {labels.active}
        </label>
        <label className="seg-opt">
          <input type="radio" name="libstatus" checked={libStatus === 'want'} onChange={() => setLibStatus('want')} />
          {labels.want}
        </label>
      </div>

      {allGenres.length > 0 && (
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 'var(--space-6)' }}>
          {allGenres.map((g) => (
            <span
              key={g}
              className={`tag ${activeGenres.includes(g) ? 'tag-accent' : 'tag-outline'}`}
              style={{ cursor: 'pointer' }}
              onClick={() => toggleGenre(g)}
            >
              {g}
            </span>
          ))}
        </div>
      )}

      {filtered.length > 0 ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px,1fr))', gap: 'var(--space-3)' }}>
          {filtered.map((item) => {
            const showProgress = showProgressBar && item.status === 'active';
            const isEditing = editingProgressId === item.id;
            const progressDisplay = item.total
              ? `${item.progress}${unit(item.type)} / ${item.total}${unit(item.type)}`
              : `${item.progress}${unit(item.type)}まで`;
            const progressSuffix = item.total ? `${unit(item.type)} / ${item.total}${unit(item.type)}` : `${unit(item.type)}まで`;
            return (
              <div key={item.id} className="card elev-sm">
                <div style={{ display: 'flex', gap: 'var(--space-3)' }}>
                  <div
                    style={{
                      width: 56,
                      height: 74,
                      borderRadius: 'var(--radius-sm)',
                      flex: 'none',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      fontFamily: 'var(--font-heading)',
                      fontWeight: 600,
                      fontSize: 20,
                      background: item.color,
                      color: '#12131c',
                    }}
                  >
                    {item.initial}
                  </div>
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div className="card-title" style={{ fontSize: 15 }}>
                      {item.title}
                    </div>
                    <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap', marginTop: 4 }}>
                      {item.genres.map((g) => (
                        <span key={g} className="tag tag-neutral">
                          {g}
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
                {showProgress &&
                  (isEditing ? (
                    <div
                      style={{
                        display: 'flex',
                        alignItems: 'baseline',
                        gap: 3,
                        marginTop: 4,
                        fontSize: 12,
                        color: 'color-mix(in srgb, var(--color-text) 70%, transparent)',
                      }}
                    >
                      <input
                        className="input"
                        type="number"
                        min={0}
                        autoFocus
                        style={{
                          width: 34,
                          padding: '2px 2px',
                          fontSize: 12,
                          textAlign: 'right',
                          background: 'transparent',
                          border: 'none',
                          borderBottom: '1px solid var(--color-accent)',
                          borderRadius: 0,
                          color: 'var(--color-text)',
                        }}
                        value={item.progress}
                        onChange={(e) => setProgress(item.id, e.target.value)}
                        onBlur={() => setEditingProgressId(null)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') setEditingProgressId(null);
                        }}
                      />
                      <span>{progressSuffix}</span>
                    </div>
                  ) : (
                    <div
                      style={{ marginTop: 4, fontSize: 12, color: 'color-mix(in srgb, var(--color-text) 70%, transparent)', cursor: 'pointer' }}
                      onClick={() => setEditingProgressId(item.id)}
                    >
                      {progressDisplay}
                    </div>
                  ))}
              </div>
            );
          })}
        </div>
      ) : (
        <p className="text-muted" style={{ fontSize: 13 }}>
          該当する作品がありません。
        </p>
      )}
    </div>
  );
}
