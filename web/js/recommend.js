// recommend.js — おすすめ画面
// ログイン中ユーザーの「見た」「見てる」ジャンルを集計し、上位ジャンルの人気作品を
// AniListから取得して、ライブラリ未登録のものだけ表示する。
const recState = {
  type: 'anime',
  user: null,
};

function initRecommendPage(user) {
  recState.user = user;

  if (!user) {
    document.getElementById('rec-tabs').style.display = 'none';
    document.getElementById('rec-content').replaceChildren(
      el('p', { className: 'text-muted', style: { fontSize: '13px' } }, [
        'ログインするとあなたの視聴傾向に合わせたおすすめが表示されます。 ',
        el('a', { href: '/login.html' }, 'ログイン'),
      ])
    );
    return;
  }

  for (const input of document.querySelectorAll('#rec-tabs input[name="rectab"]')) {
    input.addEventListener('change', () => {
      recState.type = input.value;
      syncRecTabUI();
      loadRecommendations();
    });
  }

  syncRecTabUI();
  loadRecommendations();
}

function syncRecTabUI() {
  for (const opt of document.querySelectorAll('#rec-tabs .seg-opt')) {
    const input = opt.querySelector('input');
    opt.classList.toggle('checked', input.value === recState.type);
  }
}

// ライブラリ全体（除外用の登録済みID）と、「見た/見てる」分（ジャンル集計用）を1回の取得で分ける。
function topGenres(records) {
  const counts = new Map();
  for (const r of records) {
    if (r.status !== 'done' && r.status !== 'active') continue;
    for (const g of r.genres) {
      counts.set(g, (counts.get(g) || 0) + 1);
    }
  }
  return Array.from(counts.entries())
    .sort((a, b) => b[1] - a[1])
    .slice(0, 3)
    .map(([genre]) => genre);
}

async function loadRecommendations() {
  const container = document.getElementById('rec-content');
  container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '読み込み中…'));

  let library = [];
  try {
    library = await api.listRecords({ type: recState.type });
  } catch (err) {
    container.replaceChildren(el('p', { className: 'text-muted' }, `読み込みに失敗しました: ${err.message}`));
    return;
  }

  const genres = topGenres(library);
  if (genres.length === 0) {
    container.replaceChildren(
      el(
        'p',
        { className: 'text-muted', style: { fontSize: '13px' } },
        '記録がまだありません。作品を検索してライブラリに追加すると、あなたの好みに合わせたおすすめが表示されます。'
      )
    );
    return;
  }

  const registeredIds = new Set(library.map((r) => r.anilistId));

  let results = [];
  try {
    results = await api.recommendations({ type: recState.type, genres });
  } catch (err) {
    container.replaceChildren(el('p', { className: 'text-muted' }, `AniListからの取得に失敗しました: ${err.message}`));
    return;
  }

  const recommendations = results.filter((item) => !registeredIds.has(item.anilistId));
  renderRecommendPage(container, genres, recommendations);
}

function renderRecommendPage(container, genres, recommendations) {
  const genreLine = el(
    'p',
    { style: { marginBottom: 'var(--space-6)' } },
    [
      el('span', { className: 'text-muted', style: { fontSize: '12px' } }, 'あなたのよく見るジャンル：'),
      el('span', { style: { fontSize: '14px', fontWeight: '500' } }, genres.map(translateGenre).join(' ・ ')),
    ]
  );

  if (recommendations.length === 0) {
    container.replaceChildren(
      genreLine,
      el('p', { className: 'text-muted', style: { fontSize: '13px' } }, 'おすすめできる未登録の作品が見つかりませんでした。')
    );
    return;
  }

  const grid = el('div', { className: 'grid-cards' }, recommendations.map(renderRecommendCard));
  container.replaceChildren(genreLine, grid);
}

function renderRecommendCard(item) {
  const thumb = thumbEl(item, { width: '100%', height: '160px', fontSize: 24 });
  thumb.style.borderRadius = 'var(--radius-sm)';

  const genreTags = el(
    'div',
    { style: { display: 'flex', gap: '4px', flexWrap: 'wrap', marginTop: '4px' } },
    item.genres.slice(0, 2).map((g) => el('span', { className: 'tag tag-neutral' }, translateGenre(g)))
  );

  const score = formatScore(item.score);

  const footer = el('div', { style: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: '4px' } }, [
    el('div', { className: 'card-title', style: { fontSize: '14px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, item.title),
  ]);

  const children = [thumb, footer, genreTags];
  if (score) {
    children.push(el('div', { className: 'text-muted', style: { fontSize: '12px' } }, `★${score}`));
  }

  return el(
    'div',
    { className: 'card elev-sm', style: { cursor: 'pointer' }, onClick: () => openRecommendModal(item) },
    children
  );
}

function openRecommendModal(item) {
  openWorkModal({
    item,
    mediaType: recState.type,
    status: null,
    user: recState.user,
    onStatusChange: () => loadRecommendations(),
  });
}

window.authReadyPromise.then(initRecommendPage);
