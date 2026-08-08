// modal.js — 作品詳細モーダル（ホームの今季アニメカード・人気ランキングから開く）
const MODAL_STATUS_LABELS = {
  anime: { done: '見た', active: '見てる', want: '見たい' },
  manga: { done: '読んだ', active: '読んでる', want: '読みたい' },
};

let modalOverlayEl = null;

function closeWorkModal() {
  if (modalOverlayEl) {
    modalOverlayEl.remove();
    modalOverlayEl = null;
  }
}

// item: { anilistId, title, genres, synopsis, coverImageUrl, total, nextAiringAt }
// mediaType: 'anime' | 'manga'
// status: 既にライブラリにある場合の現在のステータス（'done'|'active'|'want'|null）
// user: ログイン中のユーザー（null なら未ログイン）
// onStatusChange: ステータス保存後に呼ばれるコールバック
function openWorkModal({ item, mediaType, status, user, onStatusChange }) {
  closeWorkModal();

  const labels = MODAL_STATUS_LABELS[mediaType];

  const setStatus = async (newStatus) => {
    if (!user) {
      location.href = `/login.html?next=${encodeURIComponent(location.pathname)}`;
      return;
    }
    try {
      await api.createRecord({
        anilistId: item.anilistId,
        mediaType,
        title: item.title,
        coverImageUrl: item.coverImageUrl,
        genres: item.genres,
        total: item.total,
        status: newStatus,
        nextAiringAt: item.nextAiringAt ?? null,
      });
      closeWorkModal();
      if (onStatusChange) onStatusChange(newStatus);
    } catch (err) {
      alert(`保存に失敗しました: ${err.message}`);
    }
  };

  const thumb = thumbEl(item, { width: '100%', height: '180px', fontSize: 48 });
  thumb.style.borderRadius = '0';

  const closeBtn = el(
    'button',
    {
      className: 'btn btn-icon',
      style: { position: 'absolute', top: '10px', right: '10px', background: 'var(--color-surface)' },
      onClick: () => closeWorkModal(),
    },
    '✕'
  );

  const genreTags = el(
    'div',
    { style: { display: 'flex', gap: '6px', marginBottom: '12px', flexWrap: 'wrap' } },
    (item.genres || []).map((g) => el('span', { className: 'tag tag-neutral' }, translateGenre(g)))
  );

  const makeStatusBtn = (key) =>
    el(
      'button',
      {
        className: `btn ${status === key ? 'btn-primary' : 'btn-secondary'}`,
        style: { flex: '1', fontSize: '13px' },
        onClick: () => setStatus(key),
      },
      labels[key]
    );

  const body = el('div', { style: { padding: '20px' } }, [
    el('div', { className: 'card-title', style: { fontSize: '18px', marginBottom: '6px' } }, item.title),
    genreTags,
    el(
      'p',
      {
        className: 'text-muted',
        style: {
          fontSize: '13px',
          lineHeight: '1.6',
          marginBottom: '18px',
          display: '-webkit-box',
          WebkitLineClamp: '3',
          WebkitBoxOrient: 'vertical',
          overflow: 'hidden',
        },
      },
      item.synopsis || 'あらすじは未登録です。'
    ),
    el('div', { style: { display: 'flex', gap: '8px' } }, [
      makeStatusBtn('done'),
      makeStatusBtn('active'),
      makeStatusBtn('want'),
    ]),
  ]);

  const dialog = el(
    'div',
    { className: 'card elev-lg', style: { width: '380px', maxWidth: '100%', padding: '0', overflow: 'hidden', position: 'relative' }, onClick: (e) => e.stopPropagation() },
    [
      el('div', { style: { position: 'relative' } }, [thumb, closeBtn]),
      body,
    ]
  );

  modalOverlayEl = el(
    'div',
    { className: 'modal-overlay', onClick: () => closeWorkModal() },
    [dialog]
  );
  document.body.appendChild(modalOverlayEl);
}

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeWorkModal();
});
