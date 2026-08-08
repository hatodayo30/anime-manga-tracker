// home.js — ホーム画面
// 公開エリア（ログイン不要）: 今季放送中アニメ / 全体人気ランキング
// 非公開エリア（ログイン後）: 自分が「見てる」作品の次回放送
async function renderHome(user) {
  await renderSeasonAndTrending();
  await renderWatchingSection(user);
}

async function renderSeasonAndTrending() {
  const seasonContainer = document.getElementById('season-anime-list');
  const trendingContainer = document.getElementById('trending-list');

  try {
    const [season, trending] = await Promise.all([api.seasonAnime(), api.trending()]);
    renderSeasonGrid(seasonContainer, season);
    renderTrendingList(trendingContainer, trending);
  } catch (err) {
    const message = el('p', { className: 'text-muted' }, `AniListからの取得に失敗しました: ${err.message}`);
    seasonContainer.replaceChildren(message);
    trendingContainer.replaceChildren();
  }
}

function renderSeasonGrid(container, items) {
  if (items.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '該当する作品がありません。'));
    return;
  }
  const grid = el('div', { className: 'grid-search' });
  for (const item of items) {
    const thumb = thumbEl(item, { width: '100%', height: 'auto', fontSize: 16 });
    thumb.style.aspectRatio = '1/1';
    thumb.style.borderRadius = 'var(--radius-sm)';
    grid.appendChild(
      el('div', { className: 'card elev-sm', style: { padding: 'var(--space-2)', gap: '5px' } }, [
        thumb,
        el('div', { className: 'card-title', style: { fontSize: '13px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, displayTitle(item)),
      ])
    );
  }
  container.replaceChildren(grid);
}

function renderTrendingList(container, items) {
  if (items.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '該当する作品がありません。'));
    return;
  }
  const list = el('div', { className: 'grid-watching' });
  items.forEach((item, i) => {
    const thumb = thumbEl(item, { width: '44px', height: '44px', fontSize: 16 });
    list.appendChild(
      el('div', { className: 'card elev-sm', style: { flexDirection: 'row', alignItems: 'center', gap: 'var(--space-3)' } }, [
        el('div', { className: 'tag tag-accent', style: { flex: 'none' } }, String(i + 1)),
        thumb,
        el('div', { className: 'card-title', style: { flex: '1', fontSize: '15px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, displayTitle(item)),
      ])
    );
  });
  container.replaceChildren(list);
}

async function renderWatchingSection(user) {
  const container = document.getElementById('watching-list');

  if (!user) {
    container.replaceChildren(
      el('p', { className: 'text-muted', style: { fontSize: '13px' } }, [
        'ログインすると、あなたが「見てる」作品の次回放送日時がここに表示されます。 ',
        el('a', { href: '/login.html' }, 'ログイン'),
      ])
    );
    return;
  }

  try {
    const watchingAnime = await api.listRecords({ type: 'anime', status: 'active' });
    renderWatching(container, watchingAnime);
  } catch (err) {
    container.replaceChildren(el('p', { className: 'text-muted' }, `読み込みに失敗しました: ${err.message}`));
  }
}

function renderWatching(container, items) {
  if (items.length === 0) {
    container.replaceChildren(
      el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '見てるアニメはまだありません。検索から追加できます。')
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

window.authReadyPromise.then(renderHome);
