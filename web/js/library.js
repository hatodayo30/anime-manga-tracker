// library.js — マイライブラリ画面
const libState = {
  type: 'anime',
  status: 'active',
  activeGenres: [],
  allRecords: [], // 現在の type の全ステータス分
  aniListInfo: new Map(), // anilistId -> AniList最新情報（総話数・放送状況・次話）。アニメのみ。
  pendingIds: new Set(), // 更新中のレコードID。連打で古いprogressのまま二重更新するのを防ぐ。
};

function initLibraryPage() {
  for (const input of document.querySelectorAll('#lib-type-tabs input[name="libtype"]')) {
    input.addEventListener('change', () => {
      libState.type = input.value;
      libState.activeGenres = [];
      syncTypeUI();
      loadAndRender();
    });
  }

  for (const input of document.querySelectorAll('#lib-status-tabs input[name="libstatus"]')) {
    input.addEventListener('change', () => {
      libState.status = input.value;
      syncStatusUI();
      render();
    });
  }

  syncTypeUI();
  syncStatusUI();
  loadAndRender();
}

function syncTypeUI() {
  for (const opt of document.querySelectorAll('#lib-type-tabs .seg-opt')) {
    const input = opt.querySelector('input');
    opt.classList.toggle('checked', input.value === libState.type);
  }
}

function syncStatusUI() {
  const labels = STATUS_LABELS[libState.type];
  const opts = document.querySelectorAll('#lib-status-tabs .seg-opt');
  const order = ['done', 'active', 'want'];
  opts.forEach((opt, i) => {
    const input = opt.querySelector('input');
    const key = order[i];
    input.value = key;
    opt.querySelector('.seg-label').textContent = labels[key];
    opt.classList.toggle('checked', key === libState.status);
  });
}

async function loadAndRender() {
  const container = document.getElementById('lib-results');
  try {
    libState.allRecords = await api.listRecords({ type: libState.type });
  } catch (err) {
    container.replaceChildren(el('p', { className: 'text-muted' }, `読み込みに失敗しました: ${err.message}`));
    return;
  }

  libState.aniListInfo = new Map();
  if (libState.type === 'anime' && libState.allRecords.length > 0) {
    try {
      const ids = libState.allRecords.map((r) => r.anilistId);
      const fresh = await api.mediaByIds({ type: 'anime', ids });
      libState.aniListInfo = new Map(fresh.map((f) => [f.anilistId, f]));
    } catch {
      // AniList側の取得に失敗しても、保存済みの記録だけで表示を続ける。
    }
  }

  render();
}

function render() {
  renderGenreChips();
  renderResults();
}

function renderGenreChips() {
  const container = document.getElementById('lib-genre-chips');
  const allGenres = Array.from(new Set(libState.allRecords.flatMap((r) => r.genres)));

  if (allGenres.length === 0) {
    container.replaceChildren();
    return;
  }

  const chips = allGenres.map((g) =>
    el(
      'span',
      {
        className: `tag ${libState.activeGenres.includes(g) ? 'tag-accent' : 'tag-outline'}`,
        onClick: () => {
          libState.activeGenres = libState.activeGenres.includes(g)
            ? libState.activeGenres.filter((x) => x !== g)
            : [...libState.activeGenres, g];
          render();
        },
      },
      translateGenre(g)
    )
  );
  container.replaceChildren(el('div', { style: { display: 'flex', gap: '6px', flexWrap: 'wrap' } }, chips));
}

function renderResults() {
  const container = document.getElementById('lib-results');
  let pool = libState.allRecords.filter((r) => r.status === libState.status);
  if (libState.activeGenres.length > 0) {
    pool = pool.filter((r) => r.genres.some((g) => libState.activeGenres.includes(g)));
  }

  if (pool.length === 0) {
    container.replaceChildren(el('p', { className: 'text-muted', style: { fontSize: '13px' } }, '該当する作品がありません。'));
    return;
  }

  const grid = el('div', { className: 'grid-cards' });
  for (const item of pool) {
    grid.appendChild(renderLibraryCard(item));
  }
  container.replaceChildren(grid);
}

function renderLibraryCard(item) {
  const thumb = thumbEl(item, { width: '56px', height: '74px', fontSize: 20 });
  thumb.style.borderRadius = 'var(--radius-sm)';

  const genreTags = el(
    'div',
    { style: { display: 'flex', gap: '4px', flexWrap: 'wrap', marginTop: '4px' } },
    item.genres.map((g) => el('span', { className: 'tag tag-neutral' }, translateGenre(g)))
  );

  const header = el('div', { style: { display: 'flex', gap: 'var(--space-3)' } }, [
    thumb,
    el('div', { style: { minWidth: '0', flex: '1' } }, [el('div', { className: 'card-title', style: { fontSize: '15px' } }, item.title), genreTags]),
  ]);

  const children = [header];

  if (item.status === 'active') {
    children.push(renderProgressBlock(item));
  }

  return el('div', { className: 'card elev-sm' }, children);
}

// アニメが「放送中」のとき、AniListのnextAiringEpisode（次に放送される話数）から
// 現在放送済みの話数（= 次話 - 1）を導く。
function currentAiringEpisode(freshInfo) {
  if (!freshInfo || freshInfo.airingStatus !== 'RELEASING' || freshInfo.nextEpisode == null) return null;
  return freshInfo.nextEpisode - 1;
}

function renderProgressBlock(item) {
  const unit = unitFor(item.mediaType);
  const freshInfo = libState.aniListInfo.get(item.anilistId);
  const total = freshInfo?.total ?? item.total ?? null;
  const airingEp = item.mediaType === 'anime' ? currentAiringEpisode(freshInfo) : null;

  const rows = [];

  if (airingEp != null) {
    let text = `${item.progress}${unit}視聴 / 現在${airingEp}${unit}放送中`;
    if (total) text += ` / 全${total}${unit}`;
    rows.push(el('div', { className: 'text-muted', style: { fontSize: '12px' } }, text));
  }

  const countLabel = total ? `${item.progress} / ${total}${unit}` : `${item.progress}${unit} / ？${unit}`;

  // 放送中は、まだ放送されていない話数までは視聴済みにできないよう、
  // 「＋」の上限を現在放送済み話数（airingEp）に制限する。放送終了後は総話数(total)が上限。
  const progressCap = airingEp != null ? airingEp : total;

  const pending = libState.pendingIds.has(item.id);
  const minusBtn = el(
    'button',
    {
      className: 'btn btn-secondary btn-step',
      disabled: pending || item.progress <= 0,
      onClick: () => adjustProgress(item, -1, total, progressCap),
    },
    '－'
  );
  const plusBtn = el(
    'button',
    {
      className: 'btn btn-secondary btn-step',
      disabled: pending || (progressCap != null && item.progress >= progressCap),
      onClick: () => adjustProgress(item, 1, total, progressCap),
    },
    '＋'
  );

  const stepperRow = el('div', { className: 'progress-row' }, [
    minusBtn,
    el('div', { className: 'progress-count' }, countLabel),
    plusBtn,
  ]);
  rows.push(stepperRow);

  if (total) {
    const pct = Math.min(100, Math.round((item.progress / total) * 100));
    rows.push(
      el('div', { className: 'progress-bar' }, [el('div', { className: 'progress-bar-fill', style: { width: `${pct}%` } })])
    );
    rows.push(el('div', { className: 'progress-pct' }, `${pct}%`));
  }

  return el('div', { className: 'progress-block' }, rows);
}

// progressCap: 放送中なら現在放送済み話数、それ以外は総話数（未定ならnull）。
// UIのボタンはこの上限で disabled にしているが、ここでも同じ上限で clamp しておくことで、
// 更新処理自体が上限を知らないまま呼ばれても未放送分まで視聴済みにしてしまわないようにする。
async function adjustProgress(item, delta, total, progressCap) {
  if (libState.pendingIds.has(item.id)) return;

  let n = item.progress + delta;
  if (delta > 0 && progressCap != null) n = Math.min(n, progressCap);
  n = Math.max(0, n);
  if (n === item.progress) return;

  const body = { progress: n };
  if (total != null && n >= total) {
    body.status = 'done';
  }

  libState.pendingIds.add(item.id);
  renderResults(); // ボタンを即座に disabled にして連打による二重更新を防ぐ

  try {
    await api.updateRecord(item.id, body);
  } catch (err) {
    alert(`更新に失敗しました: ${err.message}`);
    libState.pendingIds.delete(item.id);
    renderResults();
    return;
  }

  // AniListの最新情報（総話数・放送状況）は progress の変更では変わらないので、
  // ライブラリ全体とAniList情報をまるごと再取得せず、ローカルの状態だけ更新して再描画する。
  item.progress = n;
  if (body.status) item.status = body.status;
  libState.pendingIds.delete(item.id);
  renderResults();
}

window.authReadyPromise.then((user) => {
  if (!user) return; // 未ログイン: auth.js がログイン画面へリダイレクト中
  initLibraryPage();
});
