// home.js — ホーム画面: 見てるアニメ・次回放送 / 読んでる漫画
async function renderHome() {
  const watchingContainer = document.getElementById('watching-list');
  const readingContainer = document.getElementById('reading-list');

  let watchingAnime = [];
  let readingManga = [];
  try {
    [watchingAnime, readingManga] = await Promise.all([
      api.listRecords({ type: 'anime', status: 'active' }),
      api.listRecords({ type: 'manga', status: 'active' }),
    ]);
  } catch (err) {
    watchingContainer.replaceChildren(el('p', { className: 'text-muted' }, `読み込みに失敗しました: ${err.message}`));
    return;
  }

  renderWatching(watchingContainer, watchingAnime);
  renderReading(readingContainer, readingManga);
}

function renderWatching(container, items) {
  if (items.length === 0) {
    container.replaceChildren(
      el('p', { className: 'text-muted', style: { fontSize: '13px', marginBottom: 'var(--space-8)' } }, '見てるアニメはまだありません。検索から追加できます。')
    );
    return;
  }

  const list = el('div', { className: 'grid-watching' });
  for (const item of items) {
    const card = el('div', { className: 'card elev-sm', style: { flexDirection: 'row', alignItems: 'center', gap: 'var(--space-4)' } }, [
      thumbEl(item, { width: '52px', height: '52px', fontSize: 18 }),
      el('div', { style: { flex: '1', minWidth: '0' } }, [
        el('div', { className: 'card-title', style: { whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, item.title),
        el('div', { className: 'card-meta', style: { marginTop: '2px' } }, progressLabel(item)),
      ]),
      el('div', { className: 'tag tag-accent', style: { display: 'flex', alignItems: 'center', gap: '5px', flex: 'none' } }, formatCountdown(item.nextAiringAt)),
    ]);
    list.appendChild(card);
  }
  container.replaceChildren(list);
}

function renderReading(container, items) {
  if (items.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '読んでる漫画はまだありません。'));
    return;
  }

  const grid = el('div', { className: 'grid-cards' });
  for (const item of items) {
    const card = el('div', { className: 'card elev-sm' }, [
      el('div', { style: { display: 'flex', alignItems: 'center', gap: 'var(--space-3)' } }, [
        thumbEl(item, { width: '44px', height: '44px', fontSize: 16 }),
        el('div', { style: { minWidth: '0' } }, [
          el('div', { className: 'card-title', style: { fontSize: '15px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, item.title),
          el('div', { className: 'card-meta' }, progressLabel(item)),
        ]),
      ]),
    ]);
    grid.appendChild(card);
  }
  container.replaceChildren(grid);
}

document.addEventListener('DOMContentLoaded', renderHome);
