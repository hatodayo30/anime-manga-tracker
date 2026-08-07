import type { LibraryItem } from '../types';
import { formatCountdown } from '../utils/format';
import { ClockIcon } from '../components/Icons';

interface HomeProps {
  library: LibraryItem[];
}

export function Home({ library }: HomeProps) {
  const watchingAnime = library
    .filter((i) => i.type === 'anime' && i.status === 'active')
    .sort((a, b) => (a.nextAir ?? Infinity) - (b.nextAir ?? Infinity));

  const readingManga = library.filter((i) => i.type === 'manga' && i.status === 'active');

  return (
    <div style={{ maxWidth: 920 }}>
      <h2 style={{ marginBottom: 4 }}>ホーム</h2>
      <p className="text-muted" style={{ marginBottom: 'var(--space-8)', fontSize: 13 }}>
        見てる・読んでる作品の状況
      </p>

      <h5
        style={{
          color: 'var(--color-accent)',
          textTransform: 'uppercase',
          letterSpacing: '0.08em',
          fontSize: 12,
          marginBottom: 'var(--space-3)',
        }}
      >
        見てるアニメ・次回放送
      </h5>
      {watchingAnime.length > 0 ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', marginBottom: 'var(--space-8)' }}>
          {watchingAnime.map((item) => (
            <div key={item.id} className="card elev-sm" style={{ flexDirection: 'row', alignItems: 'center', gap: 'var(--space-4)' }}>
              <div
                style={{
                  width: 52,
                  height: 52,
                  borderRadius: 'var(--radius-md)',
                  flex: 'none',
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
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="card-title" style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {item.title}
                </div>
                <div className="card-meta" style={{ marginTop: 2 }}>
                  {item.progress}話 / {item.total}話
                </div>
              </div>
              <div className="tag tag-accent" style={{ display: 'flex', alignItems: 'center', gap: 5, flex: 'none' }}>
                <ClockIcon />
                {formatCountdown(item.nextAir)}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-muted" style={{ fontSize: 13, marginBottom: 'var(--space-8)' }}>
          見てるアニメはまだありません。検索から追加できます。
        </p>
      )}

      <h5
        style={{
          color: 'var(--color-accent)',
          textTransform: 'uppercase',
          letterSpacing: '0.08em',
          fontSize: 12,
          marginBottom: 'var(--space-3)',
        }}
      >
        読んでる漫画
      </h5>
      {readingManga.length > 0 ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px,1fr))', gap: 'var(--space-3)' }}>
          {readingManga.map((item) => (
            <div key={item.id} className="card elev-sm">
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
                <div
                  style={{
                    width: 44,
                    height: 44,
                    borderRadius: 'var(--radius-md)',
                    flex: 'none',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontFamily: 'var(--font-heading)',
                    fontWeight: 600,
                    fontSize: 16,
                    background: item.color,
                    color: '#12131c',
                  }}
                >
                  {item.initial}
                </div>
                <div style={{ minWidth: 0 }}>
                  <div className="card-title" style={{ fontSize: 15, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {item.title}
                  </div>
                  <div className="card-meta">{item.total ? `${item.progress}巻 / ${item.total}巻` : `${item.progress}巻まで`}</div>
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-muted" style={{ fontSize: 13 }}>
          読んでる漫画はまだありません。
        </p>
      )}
    </div>
  );
}
