// modal.js — 作品詳細モーダル（ホーム・検索・おすすめ・マイライブラリの各ポスターカードから開く）
let modalOverlayEl = null;

function closeWorkModal() {
  if (modalOverlayEl) {
    modalOverlayEl.remove();
    modalOverlayEl = null;
  }
}

// item: AniList形状の表示データ { anilistId, title, genres, synopsis, coverImageUrl, total, score, nextAiringAt }
//       （マイライブラリのカードから開く場合はRecordそのものでもよい。synopsis/scoreは無ければ非表示）
// mediaType: 'anime' | 'manga'
// record: 既にライブラリにある場合の記録（{id, status, progress, total, ...}）。未登録なら null。
// user: ログイン中のユーザー（null なら未ログイン）
// onChange: 保存/更新/削除のたびに呼ばれるコールバック（呼び出し元の一覧を更新するために使う）
function openWorkModal({ item, mediaType, record, user, onChange }) {
  closeWorkModal();

  let currentRecord = record;
  let pending = false;
  let translatedSynopsis = null;
  let translationRequested = false;
  let freshInfo = null; // AniListの最新情報（総話数・巻数・放送状況・次話）。放送話数の上限判定に使う。
  let freshInfoRequested = false;

  const thisOverlay = el('div', { className: 'modal-overlay', onClick: () => closeWorkModal() }, []);
  modalOverlayEl = thisOverlay;
  document.body.appendChild(thisOverlay);

  const notifyChange = () => {
    if (onChange) onChange();
  };

  const setStatus = async (newStatus) => {
    if (!user) {
      location.href = `/login.html?next=${encodeURIComponent(location.pathname)}`;
      return;
    }
    if (pending) return;
    pending = true;
    render();
    try {
      currentRecord = currentRecord
        ? await api.updateRecord(currentRecord.id, { status: newStatus })
        : await api.createRecord({
            anilistId: item.anilistId,
            mediaType,
            title: item.title,
            coverImageUrl: item.coverImageUrl,
            genres: item.genres,
            total: item.total,
            status: newStatus,
            nextAiringAt: item.nextAiringAt ?? null,
          });
      notifyChange();
    } catch (err) {
      alert(`保存に失敗しました: ${err.message}`);
    }
    pending = false;
    render();
  };

  const adjustProgress = async (delta, total, progressCap) => {
    if (!currentRecord || pending) return;
    let n = Math.max(0, currentRecord.progress + delta);
    if (progressCap != null) n = Math.min(n, progressCap);
    if (n === currentRecord.progress) return;

    pending = true;
    render();
    const body = { progress: n };
    if (total != null && n >= total) body.status = 'done';
    try {
      currentRecord = await api.updateRecord(currentRecord.id, body);
      notifyChange();
    } catch (err) {
      alert(`更新に失敗しました: ${err.message}`);
    }
    pending = false;
    render();
  };

  const removeFromLibrary = async () => {
    if (!currentRecord || pending) return;
    if (!confirm('この作品を記録から外しますか？')) return;
    pending = true;
    render();
    try {
      await api.deleteRecord(currentRecord.id);
      currentRecord = null;
      notifyChange();
    } catch (err) {
      alert(`削除に失敗しました: ${err.message}`);
    }
    pending = false;
    render();
  };

  const ensureFreshInfo = () => {
    if (freshInfoRequested || !item.anilistId) return;
    freshInfoRequested = true;
    api
      .mediaByIds({ type: mediaType, ids: [item.anilistId] })
      .then((results) => {
        if (modalOverlayEl !== thisOverlay || results.length === 0) return;
        freshInfo = results[0];
        render();
      })
      .catch(() => {});
  };

  // アニメが「放送中」のとき、AniListのnextAiringEpisode（次に放送される話数）から
  // 現在放送済みの話数（= 次話 - 1）を導く。まだ放送されていない話数までは視聴済みにできないようにする。
  const currentAiringEpisode = () => {
    if (mediaType !== 'anime' || !freshInfo || freshInfo.airingStatus !== 'RELEASING' || freshInfo.nextEpisode == null) return null;
    return freshInfo.nextEpisode - 1;
  };

  const ensureTranslation = () => {
    if (translationRequested || !item.synopsis) return;
    translationRequested = true;
    api
      .translate({ text: item.synopsis, target: 'ja' })
      .then((translated) => {
        if (modalOverlayEl !== thisOverlay || !translated) return;
        translatedSynopsis = translated;
        render();
      })
      .catch(() => {});
  };

  function render() {
    const labels = STATUS_LABELS[mediaType];
    const unit = unitFor();
    const total = freshInfo?.total ?? item.total ?? currentRecord?.total ?? null;
    const status = currentRecord?.status ?? null;
    const airingEp = currentAiringEpisode();
    const progressCap = airingEp != null ? airingEp : total;

    const thumb = thumbEl(item, { fontSize: 38 });
    thumb.style.width = '132px';
    thumb.style.aspectRatio = '2/3';
    thumb.style.boxShadow = 'var(--shadow-md)';

    const closeBtn = el(
      'button',
      { className: 'btn btn-icon', style: { position: 'absolute', top: '12px', right: '12px', background: 'var(--color-surface)' }, onClick: () => closeWorkModal() },
      '✕'
    );
    const banner = el(
      'div',
      { className: 'dialog-banner', style: { background: `linear-gradient(135deg, ${colorForTitle(item.title)} 0%, var(--color-surface) 92%)` } },
      [closeBtn]
    );

    const genreTags = el(
      'div',
      { style: { display: 'flex', gap: '5px', flexWrap: 'wrap', marginBottom: '12px' } },
      (item.genres || []).map((g) => el('span', { className: 'tag tag-outline', style: { fontSize: '11px' } }, translateGenre(g)))
    );

    const scoreLabel = formatScore(item.score);
    const metaParts = [mediaType === 'anime' ? 'アニメ' : '漫画', total ? `全${total}${unit}` : '連載中'];
    if (scoreLabel) metaParts.push(`★${scoreLabel}`);

    let supplementalInfo = null;
    if (airingEp != null) {
      supplementalInfo = `現在${airingEp}${unit}放送中`;
    } else if (mediaType === 'manga' && freshInfo?.volumes) {
      supplementalInfo = `既刊${freshInfo.volumes}巻`;
    }

    const topRow = el('div', { style: { display: 'flex', gap: 'var(--space-6)', padding: '0 var(--space-6) var(--space-6)', marginTop: '-56px' } }, [
      el('div', { style: { width: '132px', flex: 'none' } }, [thumb]),
      el('div', { style: { flex: '1', minWidth: '0', paddingTop: '60px' } }, [
        el('h3', { style: { margin: '0 0 6px', fontSize: '21px', lineHeight: '1.3' } }, item.title),
        el('div', { className: 'text-muted', style: { fontSize: '12px', marginBottom: '10px' } }, [metaParts.join('・'), supplementalInfo ? ` ・ ${supplementalInfo}` : '']),
        genreTags,
        el(
          'p',
          { style: { fontSize: '13px', lineHeight: '1.75', margin: '0', display: '-webkit-box', WebkitLineClamp: '4', WebkitBoxOrient: 'vertical', overflow: 'hidden' } },
          translatedSynopsis || item.synopsis || 'あらすじは未登録です。'
        ),
      ]),
    ]);

    const statusBtn = (key) =>
      el(
        'button',
        { className: `status-btn ${status === key ? 'on' : ''}`, disabled: pending, onClick: () => setStatus(key) },
        status === key ? `✓ ${labels[key]}` : labels[key]
      );

    const recordHeader = el('div', { style: { display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: '10px' } }, [
      el('span', { className: 'text-muted', style: { fontSize: '12px' } }, '記録する'),
      currentRecord ? el('a', { href: 'javascript:void(0)', style: { fontSize: '11px' }, onClick: removeFromLibrary }, '記録から外す') : null,
    ]);

    const bottomChildren = [
      recordHeader,
      el('div', { style: { display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: '8px' } }, [statusBtn('done'), statusBtn('active'), statusBtn('want')]),
    ];

    if (status === 'active' && currentRecord) {
      const progress = currentRecord.progress;
      const pct = total ? Math.min(100, Math.round((progress / total) * 100)) : 0;
      bottomChildren.push(
        el('div', { style: { marginTop: 'var(--space-4)', display: 'flex', alignItems: 'center', gap: 'var(--space-4)', flexWrap: 'wrap' } }, [
          el('span', { className: 'text-muted', style: { fontSize: '12px' } }, '進捗'),
          el('div', { style: { display: 'flex', alignItems: 'center', gap: '10px' } }, [
            el('button', { className: 'btn btn-icon', disabled: pending || progress <= 0, onClick: () => adjustProgress(-1, total, progressCap) }, '－'),
            el(
              'span',
              { style: { fontFamily: 'var(--font-heading)', fontSize: '17px', fontWeight: '600', minWidth: '86px', textAlign: 'center' } },
              total ? `${progress} / ${total}${unit}` : `${progress}${unit} / ？${unit}`
            ),
            el('button', { className: 'btn btn-icon', disabled: pending || (progressCap != null && progress >= progressCap), onClick: () => adjustProgress(1, total, progressCap) }, '＋'),
          ]),
          total
            ? el('div', { style: { flex: '1', minWidth: '120px', height: '4px', background: 'var(--color-neutral-800)', borderRadius: '2px', overflow: 'hidden' } }, [
                el('div', { style: { height: '100%', background: 'var(--color-accent)', width: `${pct}%` } }),
              ])
            : null,
        ])
      );
    }

    if (currentRecord) {
      bottomChildren.push(
        el('div', { style: { marginTop: 'var(--space-4)', fontSize: '12px', color: 'var(--color-accent)' } }, `✓ 「${labels[currentRecord.status]}」に記録しました`)
      );
    }

    const bottomSection = el('div', { style: { padding: '0 var(--space-6) var(--space-6)' } }, [
      el('div', { style: { borderTop: '1px solid var(--color-divider)', paddingTop: 'var(--space-6)' } }, bottomChildren),
    ]);

    const dialog = el(
      'div',
      { className: 'card elev-lg', style: { width: '660px', maxWidth: '100%', maxHeight: '90vh', overflowY: 'auto', padding: '0', position: 'relative' }, onClick: (e) => e.stopPropagation() },
      [banner, topRow, bottomSection]
    );

    modalOverlayEl.replaceChildren(dialog);
  }

  render();
  ensureTranslation();
  ensureFreshInfo();
}

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeWorkModal();
});
