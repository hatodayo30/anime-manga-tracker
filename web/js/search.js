// search.js — 検索・追加画面
const state = {
  tab: 'anime',
  query: '',
};

let debounceTimer = null;

function initSearchPage() {
  const tabButtons = document.querySelectorAll('#search-tabs input[name="searchtab"]');
  for (const input of tabButtons) {
    input.addEventListener('change', () => {
      state.tab = input.value;
      syncTabUI();
      runSearch();
    });
  }

  const queryInput = document.getElementById('search-query');
  queryInput.addEventListener('input', () => {
    state.query = queryInput.value;
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(runSearch, 300);
  });

  syncTabUI();
  runSearch();
}

function syncTabUI() {
  for (const opt of document.querySelectorAll('#search-tabs .seg-opt')) {
    const input = opt.querySelector('input');
    opt.classList.toggle('checked', input.value === state.tab);
  }
}

async function runSearch() {
  const container = document.getElementById('search-results');
  const { tab, query } = state;

  let results = [];
  let libraryByAniListId = new Map();
  try {
    const [searchResults, libraryRecords] = await Promise.all([
      api.searchAniList({ type: tab, q: query }),
      api.listRecords({ type: tab }),
    ]);
    // 検索前（クエリ未入力）は AniList の人気順トップ5をデフォルト表示する。
    results = query.trim() === '' ? searchResults.slice(0, 5) : searchResults;
    libraryByAniListId = new Map(libraryRecords.map((r) => [r.anilistId, r]));
  } catch (err) {
    container.replaceChildren(el('p', { className: 'text-muted' }, `検索に失敗しました: ${err.message}`));
    return;
  }

  renderResults(container, results, libraryByAniListId);
}

function renderResults(container, results, libraryByAniListId) {
  if (results.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '該当する作品が見つかりません。'));
    return;
  }

  const labels = STATUS_LABELS[state.tab];
  const grid = el('div', { className: 'grid-search' });

  for (const item of results) {
    const existing = libraryByAniListId.get(item.anilistId);
    const currentStatus = existing ? existing.status : null;

    const thumb = thumbEl(item, { width: '100%', height: 'auto', fontSize: 18 });
    thumb.style.aspectRatio = '1/1';
    thumb.style.borderRadius = 'var(--radius-sm)';

    const genreTags = el(
      'div',
      { style: { display: 'flex', gap: '4px', flexWrap: 'wrap' } },
      item.genres.map((g) => el('span', { className: 'tag tag-neutral', style: { fontSize: '10px', padding: '2px 7px' } }, translateGenre(g)))
    );

    const makeStatusBtn = (status, label) =>
      el(
        'button',
        {
          className: `btn ${currentStatus === status ? 'btn-primary' : 'btn-secondary'}`,
          style: { flex: '1', fontSize: '11px', padding: '5px 2px' },
          onClick: () => addToLibrary(item, status),
        },
        label
      );

    const buttons = el('div', { style: { display: 'flex', gap: '4px' } }, [
      makeStatusBtn('done', labels.done),
      makeStatusBtn('active', labels.active),
      makeStatusBtn('want', labels.want),
    ]);

    const card = el('div', { className: 'card elev-sm', style: { padding: 'var(--space-2)', gap: '5px' } }, [
      thumb,
      el('div', { className: 'card-title', style: { fontSize: '13px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, displayTitle(item)),
      genreTags,
      buttons,
    ]);
    grid.appendChild(card);
  }

  container.replaceChildren(grid);
}

async function addToLibrary(item, status) {
  try {
    await api.createRecord({
      anilistId: item.anilistId,
      mediaType: state.tab,
      title: displayTitle(item),
      coverImageUrl: item.coverImageUrl,
      genres: item.genres,
      total: item.total,
      status,
      nextAiringAt: item.nextAiringAt ?? null,
    });
    runSearch();
  } catch (err) {
    alert(`追加に失敗しました: ${err.message}`);
  }
}

window.authReadyPromise.then((user) => {
  if (!user) return; // 未ログイン: auth.js がログイン画面へリダイレクト中
  initSearchPage();
});
