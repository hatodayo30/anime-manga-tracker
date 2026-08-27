// search.js — 検索・追加画面
const state = {
  kind: 'anime',
  query: '',
  user: null,
  isComposing: false, // IME変換中（未確定）かどうか
  requestSeq: 0, // 直近に発行したリクエストの通し番号。古いレスポンスの描画を無視するために使う。
};

let debounceTimer = null;

function scheduleSearch(delayMs = 500) {
  clearTimeout(debounceTimer);
  debounceTimer = setTimeout(runSearch, delayMs);
}

function initSearchPage(user) {
  state.user = user;
  state.kind = getKind();
  initKindToggle((kind) => {
    state.kind = kind;
    runSearch();
  });

  const queryInput = document.getElementById('search-query');
  queryInput.placeholder = state.kind === 'anime' ? 'アニメを検索' : '漫画を検索';

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

  runSearch();
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
  const { kind, query } = state;
  const trimmed = query.trim();
  document.getElementById('search-query').placeholder = kind === 'anime' ? 'アニメを検索' : '漫画を検索';

  // IME連打やタブ切替の連打で複数のリクエストが同時に飛んだ場合、後から返ってきた
  // 古いレスポンスで新しいレスポンスを上書きしてしまわないよう、通し番号で判定する。
  const seq = ++state.requestSeq;

  if (trimmed === '') {
    await renderIdleState(seq);
    return;
  }

  const container = document.getElementById('search-results');

  // AniListのタイトルは漢字/ローマ字/英語のみでひらがな・カタカナ表記を持たないため、
  // 純粋なかな入力の場合はローマ字に変換した検索も並行して行い、結果をマージする
  // （例: 「そうそう」→ヒットなし/少数、「sousou」→葬送のフリーレン がヒット）。
  // 漢字が混じっている場合（「進撃の巨人」等）は元の検索で既に漢字タイトルに直接ヒットするため、
  // ローマ字変換（漢字部分はそのまま通過するので意味を成さない）は行わない。
  const romaji = containsKana(trimmed) && !containsKanji(trimmed) ? kanaToRomaji(trimmed) : null;
  const shouldMergeRomaji = !!romaji && romaji !== trimmed;

  let results = [];
  let libraryByAniListId = new Map();
  try {
    const [searchResults, romajiResults, libraryRecords] = await Promise.all([
      api.searchAniList({ type: kind, q: query }),
      shouldMergeRomaji ? api.searchAniList({ type: kind, q: romaji }) : Promise.resolve([]),
      api.listRecords({ type: kind }),
    ]);

    results = shouldMergeRomaji ? mergeSearchResultsByPopularity(searchResults, romajiResults) : searchResults;
    libraryByAniListId = new Map(libraryRecords.map((r) => [r.anilistId, r]));
  } catch (err) {
    if (seq !== state.requestSeq) return; // このリクエストは既に新しい検索で上書き済み
    container.replaceChildren(el('p', { className: 'text-muted' }, `検索に失敗しました: ${err.message}`));
    return;
  }

  if (seq !== state.requestSeq) return; // このリクエストは既に新しい検索で上書き済み
  renderSearchResults(container, results, libraryByAniListId);
}

function openSearchItemModal(item, record) {
  openWorkModal({ item, mediaType: state.kind, record, user: state.user, onChange: runSearch });
}

function renderSearchResults(container, results, libraryByAniListId) {
  if (results.length === 0) {
    container.replaceChildren(
      el('div', { style: { display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--space-3)', padding: 'var(--space-8) 0', color: 'var(--color-neutral-400,var(--color-text))' } }, [
        el('p', { className: 'text-muted', style: { fontSize: '13px', margin: '0' } }, `「${state.query}」に一致する作品が見つかりません。`),
      ])
    );
    return;
  }

  const grid = el(
    'div',
    { className: 'poster-grid' },
    results.map((item) => {
      const record = libraryByAniListId.get(item.anilistId) || null;
      return posterCardEl(item, {
        badgeLabel: record ? `${STATUS_LABELS[state.kind][record.status]} ✓` : null,
        caption: (item.genres || []).slice(0, 2).map(translateGenre).join('・'),
        onClick: () => openSearchItemModal(item, record),
      });
    })
  );
  container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '12px', margin: '0 0 var(--space-3)' } }, `${results.length}件の作品`), grid);
}

async function renderIdleState(seq) {
  const container = document.getElementById('search-results');
  const { kind } = state;

  let libraryRecords = [];
  let trendResults = [];
  try {
    [libraryRecords, trendResults] = await Promise.all([
      api.listRecords({ type: kind }),
      api.searchAniList({ type: kind, q: '' }),
    ]);
  } catch (err) {
    if (seq !== state.requestSeq) return;
    container.replaceChildren(el('p', { className: 'text-muted' }, `読み込みに失敗しました: ${err.message}`));
    return;
  }
  if (seq !== state.requestSeq) return;

  const libraryByAniListId = new Map(libraryRecords.map((r) => [r.anilistId, r]));
  const tsumiWorks = libraryRecords.filter((r) => r.status === 'want');
  const trendWorks = trendResults.slice(0, 5);
  const trendTitle = kind === 'anime' ? '今季人気 TOP5' : '人気の漫画 TOP5';

  const sections = [];

  sections.push(
    el('div', { className: 'shelf' }, [
      el('h5', { className: 'shelf-h', style: { marginBottom: '4px' } }, '積み作品'),
      el('p', { className: 'text-muted shelf-desc' }, '「見たい・読みたい」に入れたまま手をつけていない作品'),
      tsumiWorks.length === 0
        ? el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '積み作品はありません。')
        : el('div', { className: 'poster-grid' }, tsumiWorks.map((r) => posterCardEl(r, { onClick: () => openSearchItemModal(r, r) }))),
    ])
  );

  sections.push(
    el('div', { className: 'shelf' }, [
      el('h5', { className: 'shelf-h' }, trendTitle),
      el(
        'div',
        { style: { display: 'flex', flexDirection: 'column', gap: '8px', maxWidth: '640px' } },
        trendWorks.map((item, i) => {
          const record = libraryByAniListId.get(item.anilistId) || null;
          const thumb = thumbEl(item, { width: '32px', height: '42px', className: 'thumb', fontSize: 13 });
          const score = formatScore(item.score);
          const children = [
            el('span', { style: { fontFamily: 'var(--font-heading)', fontWeight: '700', fontSize: '16px', color: 'var(--color-accent)', width: '18px', flex: 'none' } }, String(i + 1)),
            thumb,
            el('div', { style: { flex: '1', minWidth: '0' } }, [
              el('div', { className: 'card-title', style: { fontSize: '13px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' } }, item.title),
              el('div', { className: 'text-muted', style: { fontSize: '11px', marginTop: '2px' } }, (item.genres || []).slice(0, 2).map(translateGenre).join('・')),
            ]),
          ];
          if (record) children.push(el('span', { className: 'tag tag-accent', style: { flex: 'none' } }, `${STATUS_LABELS[kind][record.status]} ✓`));
          if (score) children.push(el('span', { className: 'text-muted', style: { fontSize: '12px', flex: 'none' } }, `★${score}`));
          return el('div', { className: 'card elev-sm', style: { flexDirection: 'row', alignItems: 'center', gap: 'var(--space-3)', padding: '9px var(--space-3)', cursor: 'pointer' }, onClick: () => openSearchItemModal(item, record) }, children);
        })
      ),
    ])
  );

  container.replaceChildren(...sections);
}

window.authReadyPromise.then((user) => {
  if (!user) return; // 未ログイン: auth.js がログイン画面へリダイレクト中
  initSearchPage(user);
});
