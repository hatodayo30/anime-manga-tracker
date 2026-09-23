// season.js — シーズンブラウジング画面（年度・季節を選んでアニメ一覧を見る）
const SEASON_LABELS = { WINTER: '冬', SPRING: '春', SUMMER: '夏', FALL: '秋' };

// AniListの慣例に合わせる（12月は翌年のWINTERシーズン扱い）。home.jsのcurrentSeason（サーバー側）と同じロジック。
function currentSeasonClient(now = new Date()) {
  const month = now.getMonth() + 1;
  const year = now.getFullYear();
  if (month === 12) return { season: 'WINTER', year: year + 1 };
  if (month <= 2) return { season: 'WINTER', year };
  if (month <= 5) return { season: 'SPRING', year };
  if (month <= 8) return { season: 'SUMMER', year };
  return { season: 'FALL', year };
}

const seasonState = { user: null, season: 'WINTER', year: 2020, results: [], library: [] };

function initYearSelect() {
  const select = document.getElementById('season-year-select');
  const currentYear = new Date().getFullYear();
  const years = [];
  for (let y = currentYear; y >= 2020; y--) years.push(y);
  select.replaceChildren(
    ...years.map((y) => el('option', { value: String(y) }, `${y}年`))
  );
}

function openSeasonItemModal(item) {
  const record = seasonState.library.find((r) => r.anilistId === item.anilistId) || null;
  openWorkModal({ item, mediaType: 'anime', record, user: seasonState.user, onChange: loadSeason });
}

async function loadSeason() {
  const { season, year } = seasonState;
  document.getElementById('season-heading').textContent = `${year}年 ${SEASON_LABELS[season]}アニメ`;
  const container = document.getElementById('season-results');

  let results = [];
  let library = [];
  try {
    [results, library] = await Promise.all([
      api.seasonAnime({ season, year }),
      seasonState.user ? api.listRecords({ type: 'anime' }) : Promise.resolve([]),
    ]);
  } catch (err) {
    container.replaceChildren(el('p', { className: 'text-muted' }, `AniListからの取得に失敗しました: ${err.message}`));
    return;
  }

  seasonState.results = results;
  seasonState.library = library;

  if (results.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted' }, '該当するアニメが見つかりませんでした。'));
    return;
  }

  const libraryByAniListId = new Map(library.map((r) => [r.anilistId, r]));
  container.replaceChildren(
    ...results.map((item) => {
      const record = libraryByAniListId.get(item.anilistId) || null;
      const score = formatScore(item.score);
      return posterCardEl(item, {
        badgeLabel: record ? `${STATUS_LABELS.anime[record.status]} ✓` : null,
        caption: [score ? `★${score}` : null, ...(item.genres || []).slice(0, 2).map(translateGenre)].filter(Boolean).join('・'),
        onClick: () => openSeasonItemModal(item),
      });
    })
  );
}

function initSeasonPage(user) {
  seasonState.user = user;

  const { season, year } = currentSeasonClient();
  seasonState.season = season;
  seasonState.year = year;

  initYearSelect();
  const yearSelect = document.getElementById('season-year-select');
  const seasonSelect = document.getElementById('season-select');
  yearSelect.value = String(year);
  seasonSelect.value = season;

  yearSelect.addEventListener('change', () => {
    seasonState.year = Number(yearSelect.value);
    loadSeason();
  });
  seasonSelect.addEventListener('change', () => {
    seasonState.season = seasonSelect.value;
    loadSeason();
  });

  loadSeason();
}

window.authReadyPromise.then(initSeasonPage);
