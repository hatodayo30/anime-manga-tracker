import { useState } from 'react';
import type { ItemType, LibraryItem, Status } from '../types';
import { STATUS_LABELS } from '../data/sampleData';

interface SearchProps {
  library: LibraryItem[];
  setStatus: (id: number, status: Status) => void;
}

export function Search({ library, setStatus }: SearchProps) {
  const [searchTab, setSearchTab] = useState<ItemType>('anime');
  const [searchQuery, setSearchQuery] = useState('');

  const pool = library.filter((i) => i.type === searchTab);
  const q = searchQuery.trim();
  const results = q ? pool.filter((i) => i.title.includes(q)) : pool;
  const labels = STATUS_LABELS[searchTab];

  return (
    <div style={{ maxWidth: 920 }}>
      <h2 style={{ marginBottom: 4 }}>検索・追加</h2>
      <p className="text-muted" style={{ marginBottom: 'var(--space-6)', fontSize: 13 }}>
        作品名を入力して記録に追加
      </p>

      <div className="seg" style={{ marginBottom: 'var(--space-4)' }}>
        <label className="seg-opt">
          <input type="radio" name="searchtab" checked={searchTab === 'anime'} onChange={() => setSearchTab('anime')} />
          アニメ
        </label>
        <label className="seg-opt">
          <input type="radio" name="searchtab" checked={searchTab === 'manga'} onChange={() => setSearchTab('manga')} />
          漫画
        </label>
      </div>

      <div className="field" style={{ marginBottom: 'var(--space-6)', maxWidth: 420 }}>
        <input
          className="input"
          type="text"
          placeholder="作品名で検索（例: 蒼穹）"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
      </div>

      {results.length > 0 ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(102px,1fr))', gap: 'var(--space-2)' }}>
          {results.map((item) => {
            const btnClass = (k: Status) => (item.status === k ? 'btn-primary' : 'btn-secondary');
            return (
              <div key={item.id} className="card elev-sm" style={{ padding: 'var(--space-2)', gap: 5 }}>
                <div
                  style={{
                    width: '100%',
                    aspectRatio: '1/1',
                    borderRadius: 'var(--radius-sm)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontFamily: 'var(--font-heading)',
                    fontWeight: 600,
                    fontSize: 18,
                    background: item.color,
                    color: '#12131c',
                  }}
                >
                  {item.initial}
                </div>
                <div className="card-title" style={{ fontSize: 13, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {item.title}
                </div>
                <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                  {item.genres.map((g) => (
                    <span key={g} className="tag tag-neutral" style={{ fontSize: 10, padding: '2px 7px' }}>
                      {g}
                    </span>
                  ))}
                </div>
                <div style={{ display: 'flex', gap: 4 }}>
                  <button className={`btn ${btnClass('done')}`} style={{ flex: 1, fontSize: 11, padding: '5px 2px' }} onClick={() => setStatus(item.id, 'done')}>
                    {labels.done}
                  </button>
                  <button className={`btn ${btnClass('active')}`} style={{ flex: 1, fontSize: 11, padding: '5px 2px' }} onClick={() => setStatus(item.id, 'active')}>
                    {labels.active}
                  </button>
                  <button className={`btn ${btnClass('want')}`} style={{ flex: 1, fontSize: 11, padding: '5px 2px' }} onClick={() => setStatus(item.id, 'want')}>
                    {labels.want}
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <p className="text-muted" style={{ fontSize: 13 }}>
          該当する作品が見つかりません。
        </p>
      )}
    </div>
  );
}
