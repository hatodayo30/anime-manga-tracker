// recommend.js — おすすめ画面
// ログイン中ユーザーの「見た」「見てる」ジャンルを集計し、上位ジャンルの人気作品を
// AniListから取得して、ライブラリ未登録のものだけ複数の理由別シェルフで表示する。
const recState = { kind: 'anime', user: null };

function initRecommendPage(user) {
  recState.user = user;
  if (!user) {
    document.getElementById('kind-toggle').style.display = 'none';
    document.getElementById('rec-content').replaceChildren(
      el('p', { className: 'text-muted', style: { fontSize: '13px' } }, [
        'ログインするとあなたの視聴傾向に合わせたおすすめが表示されます。 ',
        el('a', { href: '/login.html' }, 'ログイン'),
      ])
    );
    return;
  }

  recState.kind = getKind();
  initKindToggle((kind) => {
    recState.kind = kind;
    loadAndRender();
  });
  loadAndRender();
}

function topGenres(records) {
  const counts = new Map();
  for (const r of records) {
    if (r.status !== 'done' && r.status !== 'active') continue;
    for (const g of r.genres) counts.set(g, (counts.get(g) || 0) + 1);
  }
  return Array.from(counts.entries())
    .sort((a, b) => b[1] - a[1])
    .slice(0, 3)
    .map(([genre]) => genre);
}

async function loadAndRender() {
  const container = document.getElementById('rec-content');
  container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '読み込み中…'));

  const { kind } = recState;
  let library = [];
  try {
    library = await api.listRecords({ type: kind });
  } catch (err) {
    container.replaceChildren(el('p', { className: 'text-muted' }, `読み込みに失敗しました: ${err.message}`));
    return;
  }

  const genres = topGenres(library);
  const registeredIds = new Set(library.map((r) => r.anilistId));
  const tsumi = library.filter((r) => r.status === 'want');

  let genreBased = [];
  if (genres.length > 0) {
    try {
      genreBased = (await api.recommendations({ type: kind, genres })).filter((item) => !registeredIds.has(item.anilistId));
    } catch {
      genreBased = [];
    }
  }
  const highScore = genreBased.filter((item) => item.score != null && item.score >= 80).sort((a, b) => b.score - a.score);

  const shelves = [];
  if (genreBased.length > 0) {
    shelves.push({
      title: 'いま記録している作品と近い雰囲気',
      reason: `記録済みのジャンル（${genres.map(translateGenre).join('・')}）から`,
      items: genreBased.slice(0, 10),
    });
  }
  if (highScore.length > 0) {
    shelves.push({ title: '評価が高い定番', reason: '★8.0以上・未記録の作品', items: highScore.slice(0, 10) });
  }
  if (tsumi.length > 0) {
    shelves.push({ title: '積み作品から', reason: '「見たい・読みたい」に入れたまま止まっている作品', items: tsumi, isLibraryItems: true });
  }

  if (shelves.length === 0) {
    container.replaceChildren(
      el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '記録がまだありません。作品を検索してライブラリに追加すると、あなたの好みに合わせたおすすめが表示されます。')
    );
    return;
  }

  container.replaceChildren(...shelves.map(renderShelf));
}

function renderShelf(shelf) {
  return el('div', { className: 'shelf' }, [
    el('h5', { className: 'shelf-h', style: { marginBottom: '4px' } }, shelf.title),
    el('p', { className: 'text-muted shelf-desc' }, shelf.reason),
    el(
      'div',
      { className: 'poster-row' },
      shelf.items.map((item) => {
        const record = shelf.isLibraryItems ? item : null;
        const score = formatScore(item.score);
        return posterCardEl(item, {
          caption: score ? `★${score}` : null,
          onClick: () => openWorkModal({ item, mediaType: recState.kind, record, user: recState.user, onChange: loadAndRender }),
        });
      })
    ),
  ]);
}

window.authReadyPromise.then(initRecommendPage);
