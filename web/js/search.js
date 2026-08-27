// search.js — 検索・追加画面
const state = {
  tab: 'anime',
  query: '',
  isComposing: false, // IME変換中（未確定）かどうか
  requestSeq: 0, // 直近に発行したリクエストの通し番号。古いレスポンスの描画を無視するために使う。
};

let debounceTimer = null;

function scheduleSearch(delayMs = 500) {
  clearTimeout(debounceTimer);
  debounceTimer = setTimeout(runSearch, delayMs);
}

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

  // IME変換中（未確定）はcompositionstart〜compositionendの間trueになる。
  // その間は検索を発火させず、「い」「いぬ」のような中間状態でAPIを叩かないようにする。
  queryInput.addEventListener('compositionstart', () => {
    state.isComposing = true;
  });
  queryInput.addEventListener('compositionend', () => {
    state.isComposing = false;
    state.query = queryInput.value;
    // IME確定直後：入力が続く可能性があるのでdebounceに乗せる（即時発火はしない）。
    scheduleSearch();
  });

  queryInput.addEventListener('input', () => {
    state.query = queryInput.value;
    if (state.isComposing) return; // 変換中の中間状態では何もしない（compositionendで別途処理）
    scheduleSearch();
  });

  queryInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !state.isComposing) {
      clearTimeout(debounceTimer);
      runSearch();
    }
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

// 同じ作品（anilistId）を除いた上で、AniListの人気値（popularity）降順にまとめる。
// ひらがな/カタカナ入力時、元のかな検索とローマ字変換検索の両方を実行して結果をマージするために使う。
function mergeSearchResultsByPopularity(a, b) {
  const byId = new Map();
  for (const item of [...a, ...b]) {
    if (!byId.has(item.anilistId)) byId.set(item.anilistId, item);
  }
  return Array.from(byId.values()).sort((x, y) => (y.popularity ?? 0) - (x.popularity ?? 0));
}

async function runSearch() {
  const container = document.getElementById('search-results');
  const { tab, query } = state;
  const trimmed = query.trim();

  // IME連打やタブ切替の連打で複数のリクエストが同時に飛んだ場合、後から返ってきた
  // 古いレスポンスで新しいレスポンスを上書きしてしまわないよう、通し番号で判定する。
  const seq = ++state.requestSeq;

  // AniListのタイトルは漢字/ローマ字/英語のみでひらがな・カタカナ表記を持たないため、
  // 純粋なかな入力の場合はローマ字に変換した検索も並行して行い、結果をマージする
  // （例: 「そうそう」→ヒットなし/少数、「sousou」→葬送のフリーレン がヒット）。
  // 漢字が混じっている場合（「進撃の巨人」等）は元の検索で既に漢字タイトルに直接ヒットするため、
  // ローマ字変換（漢字部分はそのまま通過するので意味を成さない）は行わない。
  const romaji = trimmed !== '' && containsKana(trimmed) && !containsKanji(trimmed) ? kanaToRomaji(trimmed) : null;
  const shouldMergeRomaji = !!romaji && romaji !== trimmed;

  let results = [];
  let libraryByAniListId = new Map();
  try {
    const [searchResults, romajiResults, libraryRecords] = await Promise.all([
      api.searchAniList({ type: tab, q: query }),
      shouldMergeRomaji ? api.searchAniList({ type: tab, q: romaji }) : Promise.resolve([]),
      api.listRecords({ type: tab }),
    ]);

    results = shouldMergeRomaji ? mergeSearchResultsByPopularity(searchResults, romajiResults) : searchResults;

    // 検索前（クエリ未入力）は AniList の人気順トップ5をデフォルト表示する。
    if (trimmed === '') results = results.slice(0, 5);

    libraryByAniListId = new Map(libraryRecords.map((r) => [r.anilistId, r]));
  } catch (err) {
    if (seq !== state.requestSeq) return; // このリクエストは既に新しい検索で上書き済み
    container.replaceChildren(el('p', { className: 'text-muted' }, `検索に失敗しました: ${err.message}`));
    return;
  }

  if (seq !== state.requestSeq) return; // このリクエストは既に新しい検索で上書き済み
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
      el('div', { className: 'card-title', style: { fontSize: '13px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, item.title),
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
      title: item.title,
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
